package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"postOffice/internal/postman"
	"postOffice/internal/servicebus"
	"postOffice/internal/workflow"
)

const watchTimeLayout = "15:04:05"

var errNoPeekRequests = fmt.Errorf("no %s requests to watch", postman.MethodServiceBusPeek)

var watchCommand = &cliCommand{
	name:      "watch",
	summary:   "Stream the messages arriving on Service Bus queues and subscriptions",
	aliasable: true,
	usage: `Usage:
  postOffice watch <collection> "<Folder/Sub/Request>" [options]
  postOffice watch <collection> --folder "<Folder/Sub>" [options]
  postOffice watch <collection> [options]
  postOffice watch ["<Folder/Sub/Request>"] [options]

Watches PEEK requests (asb://queue or asb://topic/subscription) the way the
TUI's listeners do: one request, every PEEK request in a folder, or every one
in the collection. Each starts with the newest messages already waiting (the
request's Backlog header, default 100), then prints messages as they arrive.
Peeking neither locks nor removes them. Variables resolve when the watch
starts; no scripts run. <collection> and request paths work as for "run".

Options:
  -e, --environment <env>   environment file or loaded environment name to
                            resolve variables from (default: the selected one)
  --folder <Folder/Sub>     watch every PEEK request in this folder
  --output text|json        text (default): one line per message, plus failed
                            polls and recoveries; json: one JSON object per
                            line, a message (as in the TUI's export) or a
                            failed poll ({"request","target","error","retryAt"})

A failing watch keeps retrying, waiting longer after each failure (up to 30s).
ctrl+c stops watching and exits 0. Exit code 2: usage or input error, or a
request that can't be watched.`,
	run: func(out io.Writer, args []string) error {
		opts, err := parseWatchArgs(args)
		if err != nil {
			return err
		}
		if opts, err = withSelectedCollection(opts); err != nil {
			return err
		}
		ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stopSignals()
		return executeWatch(ctx, out, opts)
	},
}

// parseWatchArgs reads a watch command line into the run options it shares
// with "run": collection, request or folder, environment and output.
func parseWatchArgs(args []string) (runOptions, error) {
	parsed, err := parseFlags(args, folderFlag, environmentFlag, outputFlag)
	if err != nil {
		return runOptions{}, err
	}
	opts := runOptions{execution: executionOptions{environmentPath: parsed.value(environmentFlag), output: outputText}}
	switch len(parsed.positional) {
	case 0:
	case 1:
		opts.collectionPath = parsed.positional[0]
	case 2:
		opts.collectionPath, opts.requestPath = parsed.positional[0], parsed.positional[1]
	default:
		return runOptions{}, usageErrorf("watch takes a collection and optionally one request path, got %d arguments", len(parsed.positional))
	}
	if parsed.has(folderFlag) {
		if opts.requestPath != "" {
			return runOptions{}, usageErrorf("give either a request path or --folder, not both")
		}
		if opts.folderPath = postman.SplitItemPath(parsed.value(folderFlag)); len(opts.folderPath) == 0 {
			return runOptions{}, usageErrorf("--folder needs a folder path such as \"Topics\"")
		}
	}
	if parsed.has(outputFlag) {
		opts.execution.output = outputFormat(parsed.value(outputFlag))
		if opts.execution.output != outputText && opts.execution.output != outputJSON {
			return runOptions{}, usageErrorf("--output must be %s or %s, got %q", outputText, outputJSON, opts.execution.output)
		}
	}
	return opts, nil
}

// watchedRequest names a watched PEEK request: its path in the collection,
// the queue or subscription it peeks and its Display-Property.
type watchedRequest struct {
	name            string
	target          string
	displayProperty string
}

// watchedUpdate is a poll result of one watched request.
type watchedUpdate struct {
	request watchedRequest
	update  servicebus.WatchUpdate
}

// executeWatch watches the PEEK requests opts selects until ctx is cancelled,
// writing what they find to out.
func executeWatch(ctx context.Context, out io.Writer, opts runOptions) error {
	execution := withSelectedEnvironment(opts.execution)
	_, collection, environment, err := loadCollectionAndEnvironment(opts.collectionPath, execution.environmentPath)
	if err != nil {
		return err
	}
	refs, err := peekRequestsToWatch(collection, opts.requestPath, opts.folderPath)
	if err != nil {
		return err
	}
	watchers, requests, err := openWatchers(collection, environment, refs)
	if err != nil {
		return err
	}

	if execution.output == outputText {
		writeWatchHeader(out, requests)
	}
	for update := range watchAll(ctx, watchers, requests) {
		if err := writeWatchUpdate(out, execution.output, update); err != nil {
			return err
		}
	}
	return nil
}

// openWatchers prepares a watch of each of refs. When one can't be watched,
// the ones already opened are closed.
func openWatchers(collection *postman.Collection, environment *postman.Environment, refs []workflow.RequestRef) ([]*servicebus.Watcher, []watchedRequest, error) {
	var watchers []*servicebus.Watcher
	var requests []watchedRequest
	for _, ref := range refs {
		watcher, err := openRequestWatcher(collection, environment, ref)
		if err != nil {
			for _, opened := range watchers {
				opened.Close()
			}
			return nil, nil, err
		}
		watchers = append(watchers, watcher)
		requests = append(requests, watchedRequest{name: ref.Path, target: watcher.Target().String(), displayProperty: watcher.DisplayProperty()})
	}
	return watchers, requests, nil
}

// openRequestWatcher prepares a watch of ref with the variables of its folder.
func openRequestWatcher(collection *postman.Collection, environment *postman.Environment, ref workflow.RequestRef) (*servicebus.Watcher, error) {
	parts := postman.SplitItemPath(ref.Path)
	variables := postman.GetAllVariables(collection, parts[:len(parts)-1], environment)
	if unresolved := postman.UnresolvedVariables(ref.Item.Request, variables); len(unresolved) > 0 {
		return nil, fmt.Errorf("cannot watch %s: unresolved %s", ref.Path, strings.Join(unresolved, ", "))
	}
	watcher, err := servicebus.OpenWatcher(ref.Item.Request, variables)
	if err != nil {
		return nil, fmt.Errorf("cannot watch %s: %w", ref.Path, err)
	}
	return watcher, nil
}

// peekRequestsToWatch lists the PEEK requests at requestPath, in folderPath,
// or (with neither) in the whole collection.
func peekRequestsToWatch(collection *postman.Collection, requestPath string, folderPath []string) ([]workflow.RequestRef, error) {
	folderPrefix := postman.JoinItemPath(folderPath...) + "/"
	var refs []workflow.RequestRef
	for _, ref := range workflow.RequestRefs(collection.Items, nil) {
		inScope := requestPath == "" && len(folderPath) == 0 ||
			requestPath != "" && ref.Path == requestPath ||
			len(folderPath) > 0 && strings.HasPrefix(ref.Path, folderPrefix)
		if inScope && ref.Item.IsServiceBusPeek() {
			refs = append(refs, ref)
		}
	}
	if len(refs) == 0 {
		scope := collection.Info.Name
		switch {
		case requestPath != "":
			scope = requestPath
		case len(folderPath) > 0:
			scope = postman.JoinItemPath(folderPath...)
		}
		return nil, fmt.Errorf("%w in %s", errNoPeekRequests, scope)
	}
	return refs, nil
}

// watchAll runs a watch per watcher and merges their updates into one
// channel, closed once every watch has stopped.
func watchAll(ctx context.Context, watchers []*servicebus.Watcher, requests []watchedRequest) <-chan watchedUpdate {
	merged := make(chan watchedUpdate)
	var running sync.WaitGroup
	for i, watcher := range watchers {
		updates := make(chan servicebus.WatchUpdate)
		go servicebus.Watch(ctx, watcher, updates)
		running.Go(func() {
			for update := range updates {
				merged <- watchedUpdate{request: requests[i], update: update}
			}
		})
	}
	go func() {
		running.Wait()
		close(merged)
	}()
	return merged
}

func writeWatchHeader(out io.Writer, requests []watchedRequest) {
	names := make([]string, len(requests))
	for i, request := range requests {
		names[i] = fmt.Sprintf("%s (%s)", request.target, request.name)
	}
	fmt.Fprintf(out, "Watching %s; ctrl+c stops\n", strings.Join(names, ", "))
}

// watchFailure is a failed poll as a JSON line.
type watchFailure struct {
	Request string    `json:"request"`
	Target  string    `json:"target"`
	Error   string    `json:"error"`
	RetryAt time.Time `json:"retryAt"`
}

func writeWatchUpdate(out io.Writer, output outputFormat, watched watchedUpdate) error {
	request, update := watched.request, watched.update
	if output == outputJSON {
		encoder := json.NewEncoder(out)
		if update.Err != nil {
			return encoder.Encode(watchFailure{Request: request.name, Target: request.target, Error: update.Err.Error(), RetryAt: update.RetryAt})
		}
		for _, message := range update.Messages {
			if err := encoder.Encode(servicebus.SourcedMessage{Request: request.name, Target: request.target, PeekedMessage: message}); err != nil {
				return err
			}
		}
		return nil
	}

	now := time.Now().Format(watchTimeLayout)
	switch {
	case update.Err != nil:
		_, err := fmt.Fprintf(out, "%s  %s  error: %v (retrying at %s)\n", now, request.target, update.Err, update.RetryAt.Format(watchTimeLayout))
		return err
	case len(update.Messages) == 0:
		_, err := fmt.Fprintf(out, "%s  %s  polling again\n", now, request.target)
		return err
	}
	for _, message := range update.Messages {
		if _, err := fmt.Fprintf(out, "%s  %s  #%d  %s  %s\n", message.EnqueuedTime.Local().Format(watchTimeLayout), request.target, message.SequenceNumber, message.Label(request.displayProperty), message.BodyPreview()); err != nil {
			return err
		}
	}
	return nil
}
