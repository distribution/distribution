FROM docs/base:latest
MAINTAINER Mary Anthony <mary@docker.com> (@moxiegirl)

# To get the git info for this repo
COPY . /src

COPY . /docs/content/registry/

# Sed to process GitHub Markdown
# 1-2 Remove comment code from metadata block
# 3 Change ](/word to ](/project/ in links
# 4 Change ](word.md) to ](/project/word)
# 5 Remove .md extension from link text
# 6 Change ](./ to ](/project/word) 
# 7 Change ](../../ to ](/project/ 
# 8 Change ](../ to ](/project/ 
# 
RUN find /docs/content/registry -type f -name "*.md" -exec sed -i.old \
    -e '/^<!.*metadata]>/g' \
    -e '/^<!.*end-metadata.*>/g' \
    -e 's/\(\]\)\([(]\)\(\/\)/\1\2\/registry\//g' \
    -e 's/\(\][(]\)\([A-Za-z0-9]*\)\(\.md\)/\1\/registry\/\2/g' \
    -e 's/\([(]\)\(.*\)\(\.md\)/\1\2/g'  \
    -e 's/\(\][(]\)\(\.\/\)/\1\/registry\//g' \
    -e 's/\(\][(]\)\(\.\.\/\.\.\/\)/\1\/registry\//g' \
    -e 's/\(\][(]\)\(\.\.\/\)/\1\/registry\//g' {} \;