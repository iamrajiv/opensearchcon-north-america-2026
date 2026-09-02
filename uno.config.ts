import { defineConfig } from 'unocss'

// Slidev generates utility classes on demand while the dev server runs, which
// the PDF/PNG exporter can capture before they arrive. Scanning these files up
// front makes the layout utilities (grid-cols-2, w-52, ...) available on the
// very first page load, so `slidev export` renders the same as the browser.
export default defineConfig({
  content: {
    filesystem: [
      'slides.md',
      'components/**/*.vue',
      'node_modules/@slidev/client/layouts/*.vue',
      'node_modules/@slidev/client/internals/*.vue',
      'node_modules/@slidev/client/builtin/*.vue',
    ],
  },
})
