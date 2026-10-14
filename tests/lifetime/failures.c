#include <node/node_api.h>

// Link-time injection: all successful calls still execute the real Node-API.
static int failure;
void fail_next(int stage) { failure = stage; }

napi_status __real_napi_set_instance_data(napi_env, void *, napi_finalize, void *);
napi_status __wrap_napi_set_instance_data(napi_env env, void *data,
    napi_finalize finalize, void *hint) {
  if (failure == 3) {
    failure = 0;
    return napi_generic_failure;
  }
  return __real_napi_set_instance_data(env, data, finalize, hint);
}

napi_status __real_napi_create_function(napi_env, const char *, size_t,
    napi_callback, void *, napi_value *);
napi_status __wrap_napi_create_function(napi_env env, const char *name,
    size_t length, napi_callback cb, void *data, napi_value *result) {
  if (failure == 1) {
    failure = 0;
    return napi_generic_failure;
  }
  return __real_napi_create_function(env, name, length, cb, data, result);
}

napi_status __real_napi_add_finalizer(napi_env, napi_value, void *,
    napi_finalize, void *, napi_ref *);
napi_status __wrap_napi_add_finalizer(napi_env env, napi_value value, void *data,
    napi_finalize finalize, void *hint, napi_ref *result) {
  if (failure == 2) {
    failure = 0;
    return napi_generic_failure;
  }
  return __real_napi_add_finalizer(env, value, data, finalize, hint, result);
}

#ifdef _WIN32
// Same delay-load convention as the native uploader consumer.
#include <windows.h>
#include <delayimp.h>
#include <string.h>
static FARPROC WINAPI node_delay_load(unsigned event, PDelayLoadInfo info) {
  if (event == dliNotePreLoadLibrary && strcmp(info->szDll, "node.exe") == 0)
    return (FARPROC)GetModuleHandle(NULL);
  return NULL;
}
PfnDliHook __pfnDliNotifyHook2 = node_delay_load;
#endif
