# Long-running streams

A stream example lexes an input that may never end, with constant memory, and
keeps lexing after corrupt data. Stream examples live under
`streams/<name>` and follow the [example standard](standard.md). Their long
tests use the public harness [`streamtest`](https://github.com/xiam/bytescan/tree/master/streamtest); the
[operations guide](https://github.com/xiam/bytescan/blob/master/docs/guides/operations.md) describes it, with the per-token
guard and `Release`. A stream example adds these rules:

- **Tests.** A `TestLongRun` lexes a generated stream with `streamtest.Run`,
  and a `TestCorruption` checks the tokens around each garbage run of
  `streamtest.Corrupt`. A driver that reads fields has a `TestReleaseBefore*`
  test with `streamtest.LexRetained`. A format with a largest length has a
  test that puts that record before the stream with `streamtest.Prefix`
  ([mqtt](../streams/mqtt),
  [syslog](../streams/syslog),
  [websocket](../streams/websocket)).
- **Budget.** The default tests of a stream example take under 3 s with and
  without `-race`, at the default budget of the harness. A long run uses the
  `-streamtest.long` flag and no test timeout:

  ```sh
  go test -timeout 0 -run Long ./streams/<name> -streamtest.long
  go test -timeout 0 ./streamtest -streamtest.long   # in bytescan: the harness self-test
  ```

  Throughput is logged, not asserted.
- **Ceiling.** An example uses the default heap ceiling of `streamtest.Run`.
  An example that holds more by design, such as a queue of tokens, sets its
  own ceiling with `WithCeiling` and states why in its README.
- **Per-token guard.** Every scanner of a stream example is built with
  `bytescan.WithMaxTokenBytes` in `newScanner` itself, so `go run` is guarded
  as the tests are: through `NewWithOptions` for one scanner, through
  `NewWithCursorOptions` for each scanner of a mode driver. The guard is
  `streamtest.TextGuard` (4,096 bytes) on a UTF-8 cursor,
  `streamtest.ByteGuard` (1,024 bytes) on the byte cursor of
  `bytesource.NewCursor`, which counts bytes, whatever their value.
- **Bounded rules.** Every rule of a stream example is bounded, so on input
  that never ends a record the example keeps producing bounded pieces and
  never reaches the guard. Every stream example except
  [streams/nmea](../streams/nmea) has a `TestGuard` that shows both sides on its
  production constructor: bounded pieces with no guard error on such input,
  and the guard error for a rule that the test adds and that grows past the
  guard.
