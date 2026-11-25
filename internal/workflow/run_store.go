package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"postOffice/internal/logger"

	internalhttp "postOffice/internal/http"
	"postOffice/internal/script"
)

const (
	runStoreFileName = ".postoffice_workflow_runs.json"

	// Runs hold resolved requests and responses (auth headers, tokens), so like
	// the request history the file is readable by the owner only.
	runStoreFileMode os.FileMode = 0o600
)

// RunKey names a workflow across collections: workflow IDs are only unique within one.
type RunKey struct {
	Collection string
	WorkflowID string
}

// RunStore keeps the latest finished run of each workflow, so a run's results
// survive a restart. A RunStore with an empty path keeps runs in memory only.
type RunStore struct {
	path string
	runs map[RunKey]ExecutionState
}

func NewRunStore(path string) *RunStore {
	return &RunStore{path: path, runs: make(map[RunKey]ExecutionState)}
}

// DefaultRunStorePath returns the run store location in the user's home directory.
func DefaultRunStorePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.LogError("WorkflowRunsDefaultPath", "UserHomeDir", err)
		return "", fmt.Errorf("resolve home directory for workflow runs: %w", err)
	}
	return filepath.Join(homeDir, runStoreFileName), nil
}

// Load replaces the in-memory runs with the persisted ones. A missing file is not an error.
func (s *RunStore) Load() error {
	runs, err := s.read()
	if err != nil {
		return err
	}
	s.runs = runs
	return nil
}

// Latest returns the latest recorded run of the workflow key names.
func (s *RunStore) Latest(key RunKey) (ExecutionState, bool) {
	state, ok := s.runs[key]
	return state, ok
}

// Record keeps state as key's latest run and persists it. The file is re-read
// first so runs recorded meanwhile by another session are not lost. The run is
// kept in memory even when persisting fails.
func (s *RunStore) Record(key RunKey, state ExecutionState) error {
	s.runs[key] = state
	if s.path == "" {
		return nil
	}
	persisted, err := s.read()
	if err != nil {
		return err
	}
	persisted[key] = state
	return s.write(persisted)
}

func (s *RunStore) read() (map[RunKey]ExecutionState, error) {
	runs := make(map[RunKey]ExecutionState)
	if s.path == "" {
		return runs, nil
	}

	logger.LogFileOpen(s.path)
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return runs, nil
		}
		logger.LogError("LoadWorkflowRuns", s.path, err)
		return nil, fmt.Errorf("read workflow runs %s: %w", s.path, err)
	}

	var records []runRecord
	if err := json.Unmarshal(data, &records); err != nil {
		logger.LogError("LoadWorkflowRuns", s.path, err)
		return nil, fmt.Errorf("parse workflow runs %s: %w", s.path, err)
	}
	for _, record := range records {
		runs[RunKey{Collection: record.Collection, WorkflowID: record.WorkflowID}] = record.state()
	}
	return runs, nil
}

func (s *RunStore) write(runs map[RunKey]ExecutionState) error {
	records := make([]runRecord, 0, len(runs))
	for key, state := range runs {
		records = append(records, newRunRecord(key, state))
	}

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		logger.LogError("SaveWorkflowRuns", s.path, err)
		return fmt.Errorf("encode workflow runs: %w", err)
	}

	logger.LogFileWrite(s.path)
	if err := os.WriteFile(s.path, data, runStoreFileMode); err != nil {
		logger.LogError("SaveWorkflowRuns", s.path, err)
		return fmt.Errorf("write workflow runs %s: %w", s.path, err)
	}
	return nil
}

// runRecord is a run as persisted. Its Steps shadow ExecutionState's to carry
// each step's response and script results, which StepState leaves out of JSON.
type runRecord struct {
	Collection string
	ExecutionState
	Steps []stepRecord
}

type stepRecord struct {
	StepState
	Response *responseRecord    `json:",omitempty"`
	Result   *script.TestResult `json:",omitempty"`
}

// responseRecord shadows Response.Error with its message: an error value does
// not survive a JSON round trip.
type responseRecord struct {
	internalhttp.Response
	Error string `json:",omitempty"`
}

func newRunRecord(key RunKey, state ExecutionState) runRecord {
	record := runRecord{Collection: key.Collection, ExecutionState: state}
	record.WorkflowID = key.WorkflowID
	for _, ss := range state.Steps {
		record.Steps = append(record.Steps, newStepRecord(ss))
	}
	return record
}

func newStepRecord(ss *StepState) stepRecord {
	record := stepRecord{StepState: *ss, Result: ss.LastResult}
	if ss.LastResponse != nil {
		record.Response = &responseRecord{Response: *ss.LastResponse}
		if ss.LastResponse.Error != nil {
			record.Response.Error = ss.LastResponse.Error.Error()
		}
	}
	return record
}

func (r runRecord) state() ExecutionState {
	state := r.ExecutionState
	state.Steps = make([]*StepState, 0, len(r.Steps))
	for _, record := range r.Steps {
		state.Steps = append(state.Steps, record.stepState())
	}
	state.IndexSteps()
	return state
}

func (r stepRecord) stepState() *StepState {
	ss := r.StepState
	ss.LastResult = r.Result
	if r.Response != nil {
		response := r.Response.Response
		if r.Response.Error != "" {
			response.Error = errors.New(r.Response.Error)
		}
		ss.LastResponse = &response
	}
	return &ss
}
