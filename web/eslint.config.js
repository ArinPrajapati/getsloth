import js from '@eslint/js';
import globals from 'globals';
import tseslint from 'typescript-eslint';

export default tseslint.config(
  {
    ignores: ['coverage/**', 'dist/**', 'eslint.config.js']
  },
  js.configs.recommended,
  ...tseslint.configs.strictTypeChecked,
  {
    languageOptions: {
      parserOptions: {
        project: './tsconfig.json',
        tsconfigRootDir: import.meta.dirname
      },
      globals: {
        ...globals.browser,
        ...globals.es2022
      }
    }
  },
  {
    files: ['src/**/*.test.ts'],
    languageOptions: {
      globals: {
        ...globals.browser,
        ...globals.es2022,
        ...globals.vitest
      }
    }
  }
);
