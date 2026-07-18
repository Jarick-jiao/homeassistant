// HomeMate Service Worker - PWA 离线缓存
var CACHE_NAME = 'homemate-v3.1.0';
var PRECACHE_URLS = ['/', '/index.html', '/manifest.json'];

// 安装：预缓存关键页面
self.addEventListener('install', function(event) {
  event.waitUntil(
    caches.open(CACHE_NAME).then(function(cache) {
      return cache.addAll(PRECACHE_URLS).catch(function() {
        // 单个资源失败不阻断安装
        return Promise.resolve();
      });
    }).then(function() {
      return self.skipWaiting();
    })
  );
});

// 激活：清理旧缓存
self.addEventListener('activate', function(event) {
  event.waitUntil(
    caches.keys().then(function(keys) {
      return Promise.all(
        keys.filter(function(k) { return k !== CACHE_NAME; })
            .map(function(k) { return caches.delete(k); })
      );
    }).then(function() {
      return self.clients.claim();
    })
  );
});

// 拦截请求：API 优先网络，静态资源缓存优先
self.addEventListener('fetch', function(event) {
  var req = event.request;
  // 仅处理 GET
  if (req.method !== 'GET') return;
  var url = new URL(req.url);

  // API 请求：网络优先（保证数据实时），失败回退缓存
  if (url.pathname.indexOf('/api/') === 0) {
    event.respondWith(
      fetch(req).catch(function() {
        return caches.match(req);
      })
    );
    return;
  }

  // 静态资源：缓存优先，失败回退网络
  event.respondWith(
    caches.match(req).then(function(cached) {
      if (cached) {
        // 后台更新缓存
        fetch(req).then(function(resp) {
          if (resp && resp.status === 200) {
            caches.open(CACHE_NAME).then(function(cache) {
              cache.put(req, resp.clone());
            });
          }
        }).catch(function() {});
        return cached;
      }
      return fetch(req).then(function(resp) {
        if (!resp || resp.status !== 200 || resp.type !== 'basic') return resp;
        var respClone = resp.clone();
        caches.open(CACHE_NAME).then(function(cache) {
          cache.put(req, respClone);
        });
        return resp;
      }).catch(function() {
        // 离线兜底：返回首页
        return caches.match('/index.html');
      });
    })
  );
});
