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

        // patch the path of esbuild
        updated[0] = `/.shopware-cli/${bundle.name}${updated[0]}`

        if (!added.length && !removed.length && updated.length === 1) {
            for (const link of document.getElementsByTagName("link")) {
                const url = new URL(link.href)

                if (url.host === location.host && url.pathname === updated[0]) {
                    const next = link.cloneNode()
                    next.href = updated[0] + '?' + Math.random().toString(36).slice(2)
                    next.onload = () => link.remove()
                    link.parentNode.insertBefore(next, link.nextSibling)
                    return
                }
            }
        }

        location.reload()
    })
}