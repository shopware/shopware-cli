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
        const allChangedFiles = [...added, ...removed, ...updated]

        // Content-addressed CSS changes are reported as one removed file and one
        // added file instead of an update. Swap the old hashed entrypoint for the
        // new one without reloading the whole administration.
        if (allChangedFiles.length && allChangedFiles.every(file => file.endsWith('.css'))) {
            const watcherPath = `/.shopware-cli/${bundle.name}`
            const nextFile = added[0] ?? updated[0]
            const previousFiles = [...removed, ...updated]
            const toWatcherPath = file => `${watcherPath}/${file.replace(/^\/+/, '')}`

            if (nextFile && added.length <= 1 && updated.length <= 1) {
                const nextPath = toWatcherPath(nextFile)
                const previousPaths = previousFiles.map(toWatcherPath)

                for (const link of document.getElementsByTagName("link")) {
                    const url = new URL(link.href)
                    const previousPath = previousPaths.find(path => url.pathname.endsWith(path))

                    if (url.host === location.host && previousPath) {
                        const next = link.cloneNode()
                        const nextUrl = new URL(link.href)
                        const pathPrefix = url.pathname.slice(0, -previousPath.length)
                        nextUrl.pathname = pathPrefix + nextPath
                        nextUrl.search = `?${Math.random().toString(36).slice(2)}`
                        next.href = nextUrl.toString()
                        next.onload = () => link.remove()
                        link.parentNode.insertBefore(next, link.nextSibling)
                        return
                    }
                }
            }
        }

        location.reload()
    })
}
