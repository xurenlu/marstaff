#!/bin/bash
find . -type f \( -name "*.go" -o -name "*.js" \) | while read file; do
  echo "$file: $(wc -l < "$file") lines"
done