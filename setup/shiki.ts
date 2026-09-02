import { defineShikiSetup } from '@slidev/types'

// Restrained syntax colors so the code reads as evidence, not decoration.
export default defineShikiSetup(() => ({
  themes: { light: 'min-light', dark: 'min-dark' },
}))
