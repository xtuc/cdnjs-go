# KV

Tools to test our Workers KV namespace.

## `upload`

Inserts packages from disk to KV. Package files and version metadata will be pushed to KV.
If the flag `-meta-only` is set, only version metadata will be pushed to KV.

```
make kv && ./bin/kv upload jquery mathjax fontawesome
```

## `upload-aggregate`

Inserts aggregate metadata to KV from scratch by scraping KV entries for package-level and version-specific metadata.

```
make kv && ./bin/kv upload-aggregate jquery mathjax fontawesome
```

## `packages`

Lists all packages in KV.

## `aggregate-packages`

Lists all packages with aggregated metadata in KV. To check each package in KV has an entry for aggregated metadata:

```
unset DEBUG && make kv && diff <(./bin/kv aggregated-packages) <(./bin/kv packages)
```

## `files`

Gets the file names stored in KV for a package.

```
make kv && ./bin/kv files jquery
```

## `meta`

Gets all metadata associated with a package in KV.

```
make kv && ./bin/kv meta jquery
```

## `aggregate`

Gets the aggregated metadata associated with a package in KV.

```
make kv && ./bin/kv aggregate jquery
```

## `purge`

Purges cache by [tag](https://developers.cloudflare.com/workers/reference/apis/cache/).

To purge cache for a new package:

```
make kv && ./bin/kv purge /packages
```

To purge cache for package-level metadata, list of versions, and aggregated metadata:

```
make kv && ./bin/kv purge a-happy-tyler
```

To purge cache of a specific version:

```
./bin/kv purge a-happy-tyler/1.0.0
```
