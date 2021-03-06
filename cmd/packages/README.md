# Packages

## `generate`

Generate package.min.js

## `set`

Upload package.min.js to Google Cloud Storage.

## `human`

Prints the human-readable JSON schema used for cdnjs/packages to STDOUT.

## `non-human`

Prints the non-human-readable JSON schema used for internal package management to STDOUT.

## `validate-human`

Validate a human-readable JSON file against the schema. Will print nothing if ok.
Pass `-missing-auto` and/or `-missing-repo` to ignore the errors when a (legacy) package does not contain `.autoupdate` or `.repository`, respectively.

To validate that all human-readable JSON files follow the schema:

```
make packages && find $BOT_BASE_PATH/packages -name '*.json' | xargs ./bin/packages -missing-auto -missing-repo validate-human
```
