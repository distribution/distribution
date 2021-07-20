#!/bin/bash -eu

sed 's/parserFuzzer/ParserFuzzer/g' -i ./configuration/fuzz.go
sed 's/fuzzParseNormalizedNamed/FuzzParseNormalizedNamed/g' -i ./reference/fuzz.go
sed 's/fuzzParseForwardedHeader/FuzzParseForwardedHeader/g' -i ./registry/api/v2/fuzz.go

compile_go_fuzzer github.com/distribution/distribution/v3/configuration ParserFuzzer parser_fuzzer
compile_go_fuzzer github.com/distribution/distribution/v3/reference FuzzParseNormalizedNamed fuzz_parsed_normalized_named
compile_go_fuzzer github.com/distribution/distribution/v3/registry/api/v2 FuzzParseForwardedHeader fuzz_parse_forwarded_header
