import { writeFileSync } from 'node:fs'
// Prevent Go's ./... discovery from traversing packages shipped by npm dependencies.
writeFileSync(new URL('../node_modules/go.mod', import.meta.url), 'module starhub-console-dependencies\n\ngo 1.26\n')
