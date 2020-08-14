<h1 align="center">
    <a href="https://cdnjs.com"><img src="https://raw.githubusercontent.com/cdnjs/brand/master/logo/standard/dark-512.png" width="175px" alt="< cdnjs >"></a>
</h1>

<h3 align="center">The #1 free and open source CDN built to make life easier for developers.</h3>

---

<p align="center">
 <a href="https://github.com/cdnjs/tools/blob/master/LICENSE">
  <img src="https://img.shields.io/badge/License-MIT-brightgreen.svg?style=flat-square" alt="MIT License">
 </a>
 <a href="https://cdnjs.discourse.group/">
  <img src="https://img.shields.io/discourse/https/cdnjs.discourse.group/status.svg?label=Community%20Discourse&style=flat-square" alt="Community">
 </a>
</p>

<p align="center">
 <a href="https://github.com/cdnjs/packages/blob/master/README.md#donate-and-support-us">
  <img src="https://img.shields.io/badge/GitHub-Sponsors-EA4AAA.svg?style=flat-square" alt="GitHub Sponsors">
 </a>
 <a href="https://opencollective.com/cdnjs">
  <img src="https://img.shields.io/badge/Open%20Collective-Support%20Us-3385FF.svg?style=flat-square" alt="Open Collective">
 </a>
 <a href="https://www.patreon.com/cdnjs">
  <img src="https://img.shields.io/badge/Patreon-Become%20a%20Patron-E95420.svg?style=flat-square" alt="Patreon">
 </a>
</p>

---

## Introduction

This repository contains various tools that we use to help with the process of maintaining cdnjs.

## Tools

- [algolia](./cmd/algolia)
- [checker](./cmd/checker)
- [packages](./cmd/packages)
- [autoupdate](./cmd/autoupdate)
- [kv](./cmd/kv)

## Configuration

- `DEBUG`: pass true to run in debug mode
- `BOT_BASE_PATH`: cdnjs home
- `SENTRY_DSN` sentry data source name (DSN)
- `WORKERS_KV_FILES_NAMESPACE_ID` workers kv namespace ID for files
- `WORKERS_KV_VERSIONS_NAMESPACE_ID` workers kv namespace ID containing metadata for versions
- `WORKERS_KV_PACKAGES_NAMESPACE_ID` workers kv namespace ID containing metadata for packages
- `WORKERS_KV_AGGREGATED_METADATA_NAMESPACE_ID` workers kv namespace ID containing aggregated metadata for packages
- `CF_ACCOUNT_ID` cloudflare kv account ID
- `CF_ZONE_ID` cloudflare kv zone ID
- `CF_API_TOKEN` cloudflare api token

## Dependencies

In `tools/` run `npm install`.

- [jpegoptim](https://www.kokkonen.net/tjko/projects.html)
- [zopflipng](https://github.com/google/zopfli)
- [brotli](https://github.com/google/brotli) (Linux)

## Setup a local environment

All the tools uses `BOT_BASE_PATH` to define a "cdnjs home".

We are going to create the home at `/tmp/cdnjs` and do the following in the directory:

- `git clone https://github.com/cdnjs/packages.git`
- `git clone https://github.com/cdnjs/glob.git`
- `mkdir -p /tmp/cdnjs/cdnjs/ajax/libs` (fake the cdnjs/cdnjs repo)

In glob run `npm install`.

Finally pass the `BOT_BASE_PATH` to the tool, for example: `BOT_BASE_PATH=/tmp/cdnjs bin/autoupdate -no-update`.

## License

Each library hosted on cdnjs is released under its own license. This cdnjs repository is published under [MIT license](LICENSE).
