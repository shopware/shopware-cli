import { writeFileSync } from 'node:fs';

async function fetchTagsPage(page: number, headers: Record<string, string>): Promise<Array<{ name: string }>> {
    const response = await fetch(`https://api.github.com/repos/shopware/shopware/tags?per_page=100&page=${page}`, {
        headers
    });
    
    if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
    }

    return await response.json() as Array<{ name: string }>;
}

async function fetchShopwareTags(): Promise<void> {
    const tags: string[] = [];
    
    try {
        const headers: Record<string, string> = {
            'User-Agent': 'Shopware-CLI Bot'
        };

        if (process.env.GITHUB_TOKEN) {
            headers['Authorization'] = `token ${process.env.GITHUB_TOKEN}`;
        }

        let page = 1;
        let hasMorePages = true;

        while (hasMorePages) {
            const data = await fetchTagsPage(page, headers);
            
            if (data.length === 0) {
                hasMorePages = false;
            } else {
                for (const tag of data) {
                    if (tag.name.startsWith('v') && tag.name.indexOf('rc') === -1 && tag.name.indexOf('RC') === -1&& tag.name.indexOf('+ea') === -1 && tag.name.indexOf('+dp') === -1) {
                        tags.push(tag.name.substring(1));
                    }
                }
                page++;
            }
        }

        if (!tags.includes('6.7.0.0')) {
            tags.unshift('6.7.0.0');
        }

        writeFileSync('versions.json', JSON.stringify(tags, null, 2));
        console.log('Successfully wrote versions.json');

    } catch (error) {
        console.error('Error:', error);
        process.exit(1);
    }
}

// Execute the function
fetchShopwareTags();
