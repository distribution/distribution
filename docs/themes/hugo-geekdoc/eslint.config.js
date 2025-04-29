import eslint from "@eslint/js";
import globals from "globals";
import babelParser from "@babel/eslint-parser";
import eslintPluginPrettierRecommended from "eslint-plugin-prettier/recommended";

export default [
    eslint.configs.recommended,
    {
        languageOptions: {
            globals: {
                ...globals.browser,
            },
            parser: babelParser,
            ecmaVersion: 2022,
            sourceType: "module",
            parserOptions: {
                requireConfigFile: false,
            },
        },
    },
    eslintPluginPrettierRecommended,
];
