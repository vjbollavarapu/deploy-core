import nextVitals from 'eslint-config-next/core-web-vitals'
import nextTs from 'eslint-config-next/typescript'

/** @type {import('eslint').Linter.Config[]} */
const config = [
  ...nextVitals,
  ...nextTs,
  {
    ignores: ['.next/**', 'node_modules/**', 'out/**'],
  },
  {
    rules: {
      '@typescript-eslint/no-explicit-any': 'error',
      '@typescript-eslint/no-unused-vars': [
        'error',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_' },
      ],
    },
  },
]

export default config
