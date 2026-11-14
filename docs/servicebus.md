# Azure Service Bus

Send messages to Service Bus topics and queues, and peek at the messages they hold, from Postman collections. Works with Azure and the local [Service Bus emulator](https://learn.microsoft.com/azure/service-bus-messaging/overview-emulator).

## Sending

A request whose URL starts with `asb://` sends its body as a message:

```json
{
  "name": "claim-created",
  "request": {
    "method": "ASB",
    "header": [
      { "key": "Connection-String", "value": "{{sb_connection_string}}" },
      { "key": "Content-Type", "value": "application/json" },
      { "key": "message-name", "value": "se.develophub.claim-documentation.claim.created.v1" }
    ],
    "body": { "mode": "raw", "raw": "{ \"id\": \"LF-26-NWGXJJ\" }" },
    "url": { "raw": "asb://{{sb_topic_claim}}" }
  }
}
```

| Header | Use |
|---|---|
| `Connection-String` | Service Bus connection string (required) |
| `Subject` | Message subject |
| `Content-Type` | Message content type |
| `Message-Id` | Message ID |
| anything else | Sent as an application property |

Pre-request scripts run before the send. A broker returns nothing to test, so test scripts don't.

## Peeking

A `PEEK` request reads the messages waiting in a queue or topic subscription **without locking or removing them**, so the subscription's consumers still get every message:

```json
{
  "name": "payments",
  "request": {
    "method": "PEEK",
    "header": [
      { "key": "Connection-String", "value": "{{sb_connection_string}}" },
      { "key": "Max-Messages", "value": "50" }
    ],
    "url": { "raw": "asb://{{sb_topic_payments}}/postoffice-tap" }
  }
}
```

The URL is `asb://<queue>` or `asb://<topic>/<subscription>`.

| Header | Use |
|---|---|
| `Connection-String` | Service Bus connection string (required) |
| `Max-Messages` | How many messages one peek returns (default 50) |
| `Dead-Letter` | `true` peeks the dead-letter queue |
| `Poll-Interval` | How often a [watch](#watching) peeks, e.g. `500ms` (default `1s`, at least `100ms`) |

`ctrl+e` (or `enter`) peeks once and shows the messages, oldest first, as a JSON array. Pre-request and test scripts run as for HTTP, so a test can check what was published:

```js
pm.test("claim created", () => {
  const names = pm.response.json().map(m => m.applicationProperties["message-name"]);
  pm.expect(names).to.include("se.develophub.claim-documentation.claim.created.v1");
});
```

Each message has `sequenceNumber`, `enqueuedTime`, `messageId`, `subject`, `contentType`, `correlationId`, `applicationProperties` and `body`. A JSON body stays JSON (`m.body.id`); any other body is a string.

Peeks work in [workflows](workflows.md) and the [headless CLI](cli.md) too. The emulator delivers a sent message to its subscriptions after a short delay, so a workflow that sends and then peeks should retry with a pause (`repeat`/`until` retries without one):

```js
await wf.run("send-claim-created");
for (let attempt = 0; attempt < 10; attempt++) {
  const result = await wf.run("peek-claims");
  if (result.response.body.includes("LF-26-NWGXJJ")) break;
  wf.sleep(500);
}
```

## Watching

Press `w` on a `PEEK` request to watch it in the background; `w` again stops it. The request row shows `◉ watching (n)` with the number of messages seen, and the TUI stays usable while listeners run. Several requests can be watched at once.

Press `W` (or `:listeners`, `:lis`) for the listeners view: each running listener, then every message seen, newest first.

| Key | Action |
|---|---|
| `j` / `k` | Navigate messages |
| `enter` | Show the message's properties and pretty-printed body |
| `c` | Clear the messages seen so far |
| `ctrl+x` | Stop every listener |
| `esc` / `q` | Back |

A watch starts with the messages already waiting, then shows only new arrivals. It resolves variables when it starts and runs no scripts. The view keeps the 500 most recent messages. A failing poll shows its error on the listener's line and the listener keeps polling, so it recovers when the broker comes back.

## Seeing every message

A peek only sees messages still in the subscription. A consumer that completes messages straight away (a worker) leaves almost nothing to peek. To see everything published to a topic, give it a subscription nothing consumes, and peek that. Messages stay until their time-to-live expires.

For the emulator, add the subscription to its config file (`servicebus-config.json`) and restart the emulator:

```json
{
  "Name": "postoffice-tap",
  "Properties": {
    "DefaultMessageTimeToLive": "PT1H",
    "LockDuration": "PT1M",
    "MaxDeliveryCount": 10,
    "DeadLetteringOnMessageExpiration": false,
    "ForwardDeadLetteredMessagesTo": "",
    "ForwardTo": ""
  }
}
```

[`examples/servicebus-listeners.json`](../examples/servicebus-listeners.json) peeks a `postoffice-tap` subscription on each topic, using [`examples/servicebus-emulator.env.json`](../examples/servicebus-emulator.env.json).
