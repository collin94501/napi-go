package main

/*
#include <node/node_api.h>
#include <stdint.h>
void fail_next(int stage);
static uintptr_t instance_token(napi_env env) {
  void *data = NULL;
  napi_get_instance_data(env, &data);
  return (uintptr_t)data;
}
*/
import "C"

import (
	"runtime"
	"runtime/cgo"
	"sync/atomic"

	"github.com/akshayganeshen/napi-go"
	"github.com/akshayganeshen/napi-go/entry"
)

var collected int32

// Inspect the existing public map to verify finalizer cleanup without adding
// production diagnostics or replacing the public CreateFunction path.
func callbackData(env napi.Env) *napi.NapiGoInstanceCallbackData {
	data := cgo.Handle(C.instance_token(C.napi_env(env))).Value().(*napi.NapiGoInstanceData)
	return &data.CallbackData
}

// Larger than the tiny allocator's objects, so finalization is observable.
type payload struct{ bytes [1024]byte }

func check(status napi.Status) {
	if status != napi.StatusOK {
		panic(napi.StatusError(status))
	}
}

func number(env napi.Env, n int32) napi.Value {
	v, status := napi.CreateDouble(env, float64(n))
	check(status)
	return v
}

func makeCallback(env napi.Env, info napi.CallbackInfo) napi.Value {
	p := &payload{}
	p.bytes[0] = 42
	runtime.SetFinalizer(p, func(*payload) { atomic.AddInt32(&collected, 1) })
	v, status := napi.CreateFunction(env, "retained", func(env napi.Env, info napi.CallbackInfo) napi.Value {
		runtime.GC()
		return number(env, int32(p.bytes[0]))
	})
	check(status)
	return v
}

func failures(env napi.Env, info napi.CallbackInfo) napi.Value {
	callbacks := callbackData(env)
	initialCount := len(callbacks.CallbackMap)
	C.fail_next(3)
	if napi.InitializeInstanceData(env) != napi.StatusGenericFailure || callbackData(env) != callbacks {
		panic("failed instance replacement invalidated the original owner")
	}
	for _, stage := range []int{1, 2} {
		C.fail_next(C.int(stage))
		p := &payload{}
		runtime.SetFinalizer(p, func(*payload) { atomic.AddInt32(&collected, 1) })
		v, status := napi.CreateFunction(env, "failed", func(env napi.Env, info napi.CallbackInfo) napi.Value {
			runtime.KeepAlive(p)
			return nil
		})
		if status != napi.StatusGenericFailure || v != nil || len(callbacks.CallbackMap) != initialCount {
			panic("callback failure did not release registration")
		}
	}
	var workData napi.NapiGoInstanceAsyncWorkData
	// A missing resource name is rejected by the real Node-API implementation.
	work, status := workData.CreateAsyncWork(env, nil, nil, func(napi.Env) {}, func(napi.Env, napi.Status) {})
	if status == napi.StatusOK || work.Handle != nil || len(workData.AsyncWorkMap) != 0 {
		panic("async creation failure did not release registration")
	}
	return number(env, 1)
}

func async(env napi.Env, info napi.CallbackInfo) napi.Value {
	promise, status := napi.CreatePromise(env)
	check(status)
	name, status := napi.CreateStringUtf8(env, "lifetime-test")
	check(status)
	var work napi.AsyncWork
	var executed int32
	work, status = napi.CreateAsyncWork(env, nil, name, func(napi.Env) {
		runtime.GC()
		atomic.StoreInt32(&executed, 42)
	}, func(env napi.Env, status napi.Status) {
		check(status)
		runtime.GC()
		check(napi.DeleteAsyncWork(env, work))
		check(napi.ResolveDeferred(env, promise.Deferred, number(env, atomic.LoadInt32(&executed))))
	})
	check(status)
	runtime.GC()
	check(napi.QueueAsyncWork(env, work))
	return promise.Value
}

func init() {
	entry.Export("makeCallback", makeCallback)
	entry.Export("failures", failures)
	entry.Export("async", async)
	entry.Export("collect", func(env napi.Env, info napi.CallbackInfo) napi.Value {
		runtime.GC()
		return number(env, atomic.LoadInt32(&collected))
	})
	entry.Export("count", func(env napi.Env, info napi.CallbackInfo) napi.Value {
		callbacks := callbackData(env)
		callbacks.Lock.RLock()
		defer callbacks.Lock.RUnlock()
		return number(env, int32(len(callbacks.CallbackMap)))
	})
	entry.Export("replaceInstance", func(env napi.Env, info napi.CallbackInfo) napi.Value {
		check(napi.InitializeInstanceData(env))
		return number(env, 1)
	})
}

func main() {}
