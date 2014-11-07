#!/bin/bash

badFiles=($(find ./ -iname "*.go" -exec gofmt -s -l {} \;))

if [ ${#badFiles[@]} -eq 0 ]; then
  echo 'Congratulations!  All Go source files are properly formatted.'
else
  {
    echo "These files are not properly gofmt'd:"
    for f in "${badFiles[@]}"; do
      echo " - $f"
    done
    echo
    echo 'Please reformat the above files using "gofmt -s -w" and commit the result.'
    echo
  } >&2
  false
fi
