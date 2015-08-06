FROM docs/base:latest
MAINTAINER Mary Anthony <mary@docker.com> (@moxiegirl)

ENV PROJECT=registry

# To get the git info for this repo
COPY . /src

COPY . /docs/content/$PROJECT/

# Processing GitHub Markdown
# 1 Remove <!--[metadata]> and <![end-metadata]-->
# 2 Remove any leading combination of dots and slashes (./, ../, ../../, etc)
# 3 Prepend /$PROJECT to all links, remove any trailing .md, only for non http:// links and non internal anchors

RUN find /docs/content/$PROJECT -type f -name "*.md" -not -name "*.compare.md" -exec sed -i.old \
    -e '/^<!\(--\)\{0,1\}\[\(end-\)\{0,1\}metadata\]\(--\)\{0,1\}>/g' \
    -e 's/\(\][(]\)\(\.*\/\)*/\1/g' \
    -e 's/\(\][(]\)\([A-Za-z0-9_/-]\{1,\}\)\(\.md\)\{0,1\}\(#\{0,1\}\(#[A-Za-z0-9_-]*\)\{0,1\}\)[)]/\1\/'$PROJECT'\/\2\4)/g' \
        {} \;

# Prepare the compare file and expect an empty diff against test.md
RUN sed -i.old -e 's/\/placeholder\//\/'$PROJECT'\//g' /docs/content/$PROJECT/test.compare.md && diff -u /docs/content/$PROJECT/test.md /docs/content/$PROJECT/test.compare.md
