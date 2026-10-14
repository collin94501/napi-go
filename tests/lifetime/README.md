# Callback lifetime regression

Run from the repository root using locally provisioned official Electron headers,
the matching runtime, Go, clang, nm and dlltool (no downloads):

```powershell
& ./tests/lifetime/run-windows.ps1 -HeadersPath .test-deps/electron-39.6.1 -RuntimePath .test-deps/electron-runtime/electron.exe
```

The runner builds a real addon with `GOEXPERIMENT=cgocheck2`,
`-gcflags=all=-d=checkptr=2` and at most two compiler jobs, then runs Electron in
Node mode with V8 GC exposed. It needs a Go toolchain supporting `cgocheck2`;
production code still uses only Go 1.18-compatible APIs. Run the script in a
dedicated PowerShell process because it sets build environment variables.
Generated files and caches stay under `.test-tmp/lifetime`.

The fixture covers 100 callbacks per environment across Go and V8 collections,
captured-object reclamation, function creation and finalizer-registration failure,
real async creation failure, async execution/completion across GC, instance-data
replacement (including failed replacement), and three worker shutdowns with live
callbacks. Link-time wrappers inject function creation, finalizer registration
and instance-data replacement failures; successful calls use the real
Node-API. Assertions check return values, registration cleanup and reclamation
of captured Go objects, including after worker teardown.

## Ownership contract

* Native callback data is an opaque integer token converted through `uintptr_t`
  in C. It is never dereferenced or treated as a Go-memory address. No C heap
  allocation or pinning is needed for these tokens.
* Each function owns a Go `runtime/cgo.Handle` containing its original callback
  map owner and ID. Go owns the map entry and callback closure. Node retains only
  the token until its function finalizer. That finalizer removes the entry and
  deletes the handle exactly once, without calling Node-API or looking up
  environment instance data. Thus instance-data replacement/finalizer ordering
  cannot invalidate the function's owner.
* Creation failure removes the entry and deletes the handle immediately. If
  finalizer registration fails, the unexposed function is discarded and the
  returned value is nil with the original Node-API failure status.
* Function creation/invocation follows the existing environment-thread contract.
  Map access remains locked. Node must not invoke a function after finalizing it.
  Normal environment shutdown invokes its finalizers; abrupt process termination
  relies on process reclamation, not on finalizers running.
* Async work retains its numeric map ID by value, not an address into Go memory.
  The existing instance map owns execute/complete closures; creation failure
  removes the entry. Existing successful-work ownership is unchanged: callers
  delete work only after execution/completion is quiescent, and must not replace
  instance data while work is active. Execute runs on a worker thread, complete
  on the environment thread. This correction does not redesign cancellation,
  async shutdown, or the existing worker-side instance-data lookup.
* Instance-data handles use the same C token conversion. Failed registration
  releases the new handle and preserves the previous one; successful replacement
  deletes the previous handle because Node does not finalize overwritten data.
* Public signatures, exported Go structs, module version and minimum Go version
  are unchanged. The token format is internal to an addon build; rebuild consumers
  against the corrected immutable dependency revision. No persisted format exists.

Human native/FFI review remains required. These tests qualify the dependency's
Node-API boundary on Windows; they do not qualify the uploader or Capture package.
Consumer integration and packaged application checks remain separate gates.
