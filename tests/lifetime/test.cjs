const assert = require('node:assert/strict');
const { Worker, isMainThread, parentPort } = require('node:worker_threads');
const addonPath = process.argv[2];
const addon = require(addonPath);

async function exercise() {
  const initialCollected = addon.collect();
  const initialCount = addon.count();
  assert.equal(addon.failures(), 1);
  for (let round = 0; round < 5; round++) {
    (() => {
      const callbacks = Array.from({ length: 20 }, () => addon.makeCallback());
      assert.equal(addon.count(), initialCount + 20);
      global.gc();
      addon.collect();
      for (const callback of callbacks) assert.equal(callback(), 42);
    })();
    await new Promise(resolve => setImmediate(resolve));
    for (let attempt = 0; attempt < 100 && addon.count() !== initialCount; attempt++) {
      global.gc();
      addon.collect();
      await new Promise(resolve => setTimeout(resolve, 10));
    }
    assert.equal(addon.count(), initialCount, 'function finalizers must remove registrations');
    assert.equal(await addon.async(), 42);
  }
  for (let attempt = 0; attempt < 100 && addon.collect() !== initialCollected + 102; attempt++) {
    global.gc();
    await new Promise(resolve => setTimeout(resolve, 10));
  }
  assert.equal(addon.collect(), initialCollected + 102, 'success and failure paths must release captured Go objects');
  // Exported callbacks must survive replacement of instance data, including
  // finalization after the original instance-data handle has been deleted.
  assert.equal(addon.replaceInstance(), 1);
  assert.equal(addon.count(), 0);
}

(async () => {
  await exercise();
  if (isMainThread) {
    for (let i = 0; i < 3; i++) {
      const initialCollected = addon.collect();
      await new Promise((resolve, reject) => {
        const worker = new Worker(__filename, { argv: [addonPath] });
        worker.on('error', reject);
        worker.on('exit', code => code === 0 ? resolve() : reject(new Error(`worker exit ${code}`)));
      });
      for (let attempt = 0; attempt < 100 && addon.collect() !== initialCollected + 103; attempt++) {
        await new Promise(resolve => setTimeout(resolve, 10));
      }
      assert.equal(addon.collect(), initialCollected + 103, 'worker teardown must release live callbacks');
    }
    console.log('PASS: callback GC, creation failures, async GC, instance replacement and worker teardown');
  } else {
    global.retainedUntilShutdown = addon.makeCallback();
    assert.equal(global.retainedUntilShutdown(), 42);
    parentPort.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
