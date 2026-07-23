// Managed by shopware-cli. Do not edit.
//
// Node preload (node --require) for the deprecated storefront webpack
// hot-reload watcher when it runs behind the shared shopware-cli proxy.
//
// The watcher's hot-reload websocket target (hostname + port) is hardcoded to
// 0.0.0.0 deep inside webpack-dev-server's options and cannot be set from any
// project file or env var, so the browser can't reach it through the proxy.
// Rather than edit the (deprecated) vendor code, this preload intercepts the
// require('webpack-dev-server') call and rewrites client.webSocketURL to the
// proxy hostname before the server starts. The vendor code runs untouched.
'use strict';

const Module = require('module');
const originalRequire = Module.prototype.require;

const host = process.env.SHOPWARE_CLI_HMR_WS_HOST;
const port = Number(process.env.SHOPWARE_CLI_HMR_WS_PORT) || 443;

if (host) {
    Module.prototype.require = function (id) {
        const loaded = originalRequire.apply(this, arguments);

        if (id !== 'webpack-dev-server') {
            return loaded;
        }

        // loaded is the WebpackDevServer class; return a subclass that fixes the
        // browser websocket target, then delegates unchanged.
        return class extends loaded {
            constructor(options, compiler) {
                options = options || {};
                options.client = options.client || {};
                options.client.webSocketURL = Object.assign(
                    {},
                    options.client.webSocketURL,
                    { hostname: host, protocol: 'wss', port: port },
                );
                super(options, compiler);
            }
        };
    };
}
