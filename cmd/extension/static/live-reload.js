let bundles;
if (Shopware.State !== undefined && Shopware.State.get('context') !== undefined) {
    bundles = Shopware.State.get('context').app.config.bundles;
} else {
    bundles = Shopware.Store.get('context').app.config.bundles;
}

for (const bundleName of Object.keys(bundles)) {
    const bundle = bundles[bundleName];

    if (bundle.liveReload !== true) {
        continue;
    }

    new EventSource(`/.shopware-cli/${bundle.name}/esbuild`).addEventListener('change', e => {
        const { added, removed, updated } = JSON.parse(e.data)

        // esbuild reports content-hashed filenames (e.g. "/my-plugin-XYZ.css") that no longer
        // match the stable "extension.css" URL the bundle is served under, so hot-swap the CSS
        // by its stable path whenever a single CSS file changed.
        if (!added.length && !removed.length && updated.length === 1 && updated[0].endsWith('.css')) {
            const cssPath = `/.shopware-cli/${bundle.name}/extension.css`

            for (const link of document.getElementsByTagName("link")) {
                const url = new URL(link.href)

                if (url.host === location.host && url.pathname === cssPath) {
                    const next = link.cloneNode()
                    next.href = cssPath + '?' + Math.random().toString(36).slice(2)
                    next.onload = () => link.remove()
                    link.parentNode.insertBefore(next, link.nextSibling)
                    return
                }
            }
        }

        location.reload()
    })
}