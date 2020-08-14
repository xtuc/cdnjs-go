package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/agnivade/levenshtein"
	"github.com/blang/semver"
	"github.com/cdnjs/tools/compress"
	"github.com/cdnjs/tools/kv"
	"github.com/cdnjs/tools/metrics"
	"github.com/cdnjs/tools/packages"
	"github.com/cdnjs/tools/sentry"
	"github.com/cdnjs/tools/util"
)

func init() {
	sentry.Init()
}

var (
	basePath     = util.GetBotBasePath()
	packagesPath = util.GetHumanPackagesPath()
	cdnjsPath    = util.GetCDNJSPath()
	logsPath     = util.GetLogsPath()

	// initialize standard debug logger
	logger = util.GetStandardLogger()

	// default context (no logger prefix)
	defaultCtx = util.ContextWithEntries(util.GetStandardEntries("", logger)...)
)

type version interface {
	Get() string             // Get the version.
	GetTimeStamp() time.Time // GetTimeStamp gets the time stamp associated with the version.
}

type newVersionToCommit struct {
	versionPath string
	newVersion  string
	pckg        *packages.Package
	timestamp   time.Time
}

// Get is used to get the new version.
func (n newVersionToCommit) Get() string {
	return n.newVersion
}

// GetTimeStamp gets the time stamp associated with the new version.
func (n newVersionToCommit) GetTimeStamp() time.Time {
	return n.timestamp
}

func main() {
	defer sentry.PanicHandler()

	var noUpdate, noPull bool
	flag.BoolVar(&noUpdate, "no-update", false, "if set, the autoupdater will not commit or push to git")
	flag.BoolVar(&noPull, "no-pull", false, "if set, the autoupdater will not pull from git")
	flag.Parse()

	if util.IsDebug() {
		fmt.Printf("Running in debug mode (no-update=%t, no-pull=%t)\n", noUpdate, noPull)
	}

	// create channel to handle signals
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGTERM)

	if !noPull {
		util.UpdateGitRepo(defaultCtx, cdnjsPath)
		util.UpdateGitRepo(defaultCtx, packagesPath)
		util.UpdateGitRepo(defaultCtx, logsPath)
	}

	for _, f := range packages.GetHumanPackageJSONFiles(defaultCtx) {
		// create context with file path prefix, standard debug logger
		ctx := util.ContextWithEntries(util.GetStandardEntries(f, logger)...)

		select {
		case sig := <-c:
			util.Debugf(ctx, "RECEIVED SIGNAL: %s\n", sig)
			return
		default:
		}

		pckg, err := packages.ReadHumanJSONFile(ctx, path.Join(packagesPath, f))
		if err != nil {
			if invalidHumanErr, ok := err.(packages.InvalidSchemaError); ok {
				for _, resErr := range invalidHumanErr.Result.Errors() {
					if resErr.String() == "(root): autoupdate is required" {
						continue // (legacy) ignore missing .autoupdate
					}
					if resErr.String() == "(root): repository is required" {
						continue // (legacy) ignore missing .repository
					}
					panic(resErr.String()) // unhandled schema problem
				}
				continue // ignore this legacy package
			}
			panic(err) // something else went wrong
		}

		var newVersionsToCommit []newVersionToCommit
		var allVersions []version

		switch *pckg.Autoupdate.Source {
		case "npm":
			{
				util.Debugf(ctx, "running npm update")
				newVersionsToCommit, allVersions = updateNpm(ctx, pckg)
			}
		case "git":
			{
				util.Debugf(ctx, "running git update")
				newVersionsToCommit, allVersions = updateGit(ctx, pckg)
			}
		default:
			{
				panic(fmt.Sprintf("%s invalid autoupdate source: %s", *pckg.Name, *pckg.Autoupdate.Source))
			}
		}

		// If there are no versions, do not write any metadata.
		if len(allVersions) <= 0 {
			continue
		}

		if !noUpdate {
			// Push new versions to git.
			newAssets, versionsChanged := updateVersions(ctx, newVersionsToCommit)

			// Update package metadata.
			pkgChanged := updatePackage(ctx, pckg, allVersions, f)

			if versionsChanged || pkgChanged {
				// Update aggregated package metadata for cdnjs API.
				pkgMetadataFound := updateAggregatedMetadata(ctx, pckg, newAssets)

				// Purge cache of this package's metadata
				// (package top-level, list of versions, aggregated metadata).
				cacheTags := []string{*pckg.Name}

				if !pkgMetadataFound {
					// This is a new package, so purge `/packages` cache tag,
					// which contains the list of packages.
					// Reference: http://metadata.speedcdnjs.com/packages
					cacheTags = append(cacheTags, "/packages")
				}

				// Purge cache so the cdnjs API will fetch updated KV entries.
				// Note, need to be careful of race conditions.
				// If cache is purged but KV values are not yet propagated globally,
				// it is possible the Worker will fetch stale entries from KV and cache them.
				util.Check(kv.PurgeTags(cacheTags))
			}
		}
	}
}

// Push new versions to git and KV.
// Returns if anything was pushed to KV.
func updateVersions(ctx context.Context, newVersionsToCommit []newVersionToCommit) ([]packages.Asset, bool) {
	var assets []packages.Asset
	var changed bool

	if len(newVersionsToCommit) > 0 {
		commitNewVersions(ctx, newVersionsToCommit)
		assets = writeNewVersionsToKV(ctx, newVersionsToCommit)
		packages.GitPush(ctx, cdnjsPath)
		packages.GitPush(ctx, logsPath)
		changed = true
	}

	return assets, changed
}

// Update package metadata in git and KV.
// Returns if the package was changed.
func updatePackage(ctx context.Context, pckg *packages.Package, allVersions []version, packageJSONPath string) bool {
	latestVersion := getLatestStableVersion(allVersions)

	if latestVersion == nil {
		latestVersion = getLatestVersion(allVersions)
	}

	// latestVersion must be non-nil by now
	// since we determined len(allVersions) > 0
	pckg.Version = latestVersion
	updateFilenameIfMissing(ctx, pckg)

	destpckg, err := kv.GetPackage(ctx, *pckg.Name)
	if err != nil {
		// check for errors
		// Note: currently panicking on unhandled errors, including AuthError
		switch e := err.(type) {
		case kv.KeyNotFoundError:
			{
				// key not found (new package)
				util.Debugf(ctx, "KV key `%s` not found, inserting package metadata...\n", *pckg.Name)
			}
		case packages.InvalidSchemaError:
			{
				// invalid schema found
				// this should not occur, so log in sentry
				// and rewrite the key so it follows the JSON schema
				sentry.NotifyError(fmt.Errorf("schema invalid for KV package metadata `%s`: %s", *pckg.Name, e))
			}
		default:
			{
				// unhandled error occurred
				panic(fmt.Sprintf("unhandled error reading KV package metadata: %s", e.Error()))
			}
		}
	} else if destpckg.Version != nil && *destpckg.Version == *pckg.Version {
		// latest version is already in KV, but we still
		// need to check if the `filename` changed or not
		if (destpckg.Filename == nil && pckg.Filename == nil) || (destpckg.Filename != nil && pckg.Filename != nil && *destpckg.Filename == *pckg.Filename) {
			return false
		}
	}

	// Either `version`, `filename` or both changed,
	// so git push the new metadata.
	commitPackageVersion(ctx, pckg, packageJSONPath)
	packages.GitPush(ctx, cdnjsPath)
	packages.GitPush(ctx, logsPath)

	if err := kv.UpdateKVPackage(ctx, pckg); err != nil {
		panic(fmt.Sprintf("failed to write KV package metadata %s: %s", *pckg.Name, err.Error()))
	}

	return true
}

type aggregatedMetadataLog struct {
	Found bool     `json:"found"`
	Keys  []string `json:"kv_writes"`
}

// Update aggregated package metadata for cdnjs API.
func updateAggregatedMetadata(ctx context.Context, pckg *packages.Package, newAssets []packages.Asset) bool {
	kvWrites, found, err := kv.UpdateAggregatedMetadata(ctx, pckg, newAssets)
	if err != nil {
		panic(fmt.Sprintf("(%s) failed to update aggregated metadata: %s", *pckg.Name, err))
	}
	if len(kvWrites) == 0 {
		panic(fmt.Sprintf("(%s) failed to update aggregated metadata (no KV writes!)", *pckg.Name))
	}

	logsJSON, err := json.Marshal(aggregatedMetadataLog{
		Found: found,
		Keys:  kvWrites,
	})
	util.Check(err)

	// Will either be ["<package name>"] or [] if the KV write fails
	packages.GitAdd(ctx, logsPath, pckg.Log("update aggregated metadata: %s: %s", *pckg.Version, logsJSON))
	logsCommitMsg := fmt.Sprintf("Set %s aggregated metadata (%s)", *pckg.Name, *pckg.Version)
	packages.GitCommit(ctx, logsPath, logsCommitMsg)
	packages.GitPush(ctx, logsPath)

	return found
}

// Update the package's filename if the latest
// version does not contain the filename
// Note that if the filename is nil it will stay nil.
func updateFilenameIfMissing(ctx context.Context, pckg *packages.Package) {
	key := pckg.LatestVersionKVKey()
	assets, err := kv.GetVersion(ctx, key)
	if err != nil {
		// All package metadata will be in KV, so this error should never occur.
		panic(fmt.Sprintf("KV metadata not found for latest version `%s`", key))
	}

	if len(assets) == 0 {
		panic(fmt.Sprintf("KV version `%s` contains no assets", key))
	}

	if pckg.Filename != nil {
		// check if assets contains filename
		filename := *pckg.Filename
		for _, asset := range assets {
			if asset == filename {
				return // filename included in latest version, so return
			}
		}

		// set filename to be the most similar string in []assets
		mostSimilar := getMostSimilarFilename(filename, assets)
		pckg.Filename = &mostSimilar
		util.Debugf(ctx, "Updated `%s` filename `%s` -> `%s`\n", key, filename, mostSimilar)
		return
	}
	util.Debugf(ctx, "Filename in `%s` missing, so will stay missing.\n", key)
}

// Gets the most similar filename to a target filename.
// The []string of alternatives must have at least one element.
func getMostSimilarFilename(target string, filenames []string) string {
	var mostSimilar string
	var minDist int = math.MaxInt32
	for _, f := range filenames {
		if dist := levenshtein.ComputeDistance(target, f); dist < minDist {
			mostSimilar = f
			minDist = dist
		}
	}
	return mostSimilar
}

// Gets the latest stable version by time stamp. A  "stable" version is
// considered to be a version that contains no pre-releases.
// If no latest stable version is found (ex. all are non-semver), a nil *string
// will be returned.
func getLatestStableVersion(versions []version) *string {
	var latest *string
	var latestTime time.Time
	for _, v := range versions {
		vStr := v.Get()
		if s, err := semver.Parse(vStr); err == nil && len(s.Pre) == 0 {
			timeStamp := v.GetTimeStamp()
			if latest == nil || timeStamp.After(latestTime) {
				latest = &vStr
				latestTime = timeStamp
			}
		}
	}
	return latest
}

// Gets the latest version by time stamp. If it does not exist, a nil *string is returned.
func getLatestVersion(versions []version) *string {
	var latest *string
	var latestTime time.Time
	for _, v := range versions {
		vStr, timeStamp := v.Get(), v.GetTimeStamp()
		if latest == nil || timeStamp.After(latestTime) {
			latest = &vStr
			latestTime = timeStamp
		}
	}
	return latest
}

// Copy the package.json to the cdnjs repo and update its version.
func updateVersionInCdnjs(ctx context.Context, pckg *packages.Package, packageJSONPath string) []byte {
	// marshal into JSON
	bytes, err := pckg.Marshal()
	util.Check(err)

	// enforce schema when writing non-human package JSON
	_, err = packages.ReadNonHumanJSONBytes(ctx, *pckg.Name, bytes)
	util.Check(err)

	// open and write to package.json file

	dest := path.Join(pckg.LibraryPath(), "package.json")
	file, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	util.Check(err)

	_, err = file.Write(bytes)
	util.Check(err)

	return bytes
}

// Optimizes/minifies files on disk for a particular package version.
func optimizeAndMinify(ctx context.Context, version newVersionToCommit) {
	files := version.pckg.AllFiles(version.newVersion)

	for _, f := range files {
		switch path.Ext(f) {
		case ".jpg", ".jpeg":
			compress.Jpeg(ctx, path.Join(version.versionPath, f))
		case ".png":
			compress.Png(ctx, path.Join(version.versionPath, f))
		case ".js":
			compress.Js(ctx, path.Join(version.versionPath, f))
		case ".css":
			compress.CSS(ctx, path.Join(version.versionPath, f))
		}
	}
}

// write all versions to KV
func writeNewVersionsToKV(ctx context.Context, newVersionsToCommit []newVersionToCommit) []packages.Asset {
	var assets []packages.Asset

	for _, newVersionToCommit := range newVersionsToCommit {
		pkg, version := *newVersionToCommit.pckg.Name, newVersionToCommit.newVersion

		util.Debugf(ctx, "writing version to KV %s", path.Join(pkg, version))
		kvVersionFiles, kvVersionMetadata, kvCompressedFiles, err := kv.InsertNewVersionToKV(ctx, pkg, version, newVersionToCommit.versionPath, false)
		if err != nil {
			panic(fmt.Sprintf("failed to write kv version %s: %s", path.Join(pkg, version), err.Error()))
		}

		kvCompressedFilesJSON, err := json.Marshal(kvCompressedFiles)
		util.Check(err)

		// Git add/commit new version to cdnjs/logs
		packages.GitAdd(ctx, logsPath, newVersionToCommit.pckg.Log("new version: %s: %s", newVersionToCommit.newVersion, kvVersionMetadata))
		packages.GitAdd(ctx, logsPath, newVersionToCommit.pckg.Log("new version kv: %s: %s", newVersionToCommit.newVersion, kvCompressedFilesJSON))
		logsCommitMsg := fmt.Sprintf("Add %s (%s)", *newVersionToCommit.pckg.Name, newVersionToCommit.newVersion)
		packages.GitCommit(ctx, logsPath, logsCommitMsg)

		metrics.ReportNewVersion(ctx)

		assets = append(assets, packages.Asset{
			Version: newVersionToCommit.newVersion,
			Files:   kvVersionFiles,
		})
	}

	return assets
}

func commitNewVersions(ctx context.Context, newVersionsToCommit []newVersionToCommit) {
	for _, newVersionToCommit := range newVersionsToCommit {
		util.Debugf(ctx, "adding version %s", newVersionToCommit.newVersion)

		// Optimize/minifiy assets (compressing br/gz will occur later)
		optimizeAndMinify(ctx, newVersionToCommit)

		// Git add/commit new version to cdnjs/cdnjs
		packages.GitAdd(ctx, cdnjsPath, newVersionToCommit.versionPath)
		commitMsg := fmt.Sprintf("Add %s (%s)", *newVersionToCommit.pckg.Name, newVersionToCommit.newVersion)
		packages.GitCommit(ctx, cdnjsPath, commitMsg)
	}
}

func commitPackageVersion(ctx context.Context, pckg *packages.Package, packageJSONPath string) {
	util.Debugf(ctx, "adding latest version to package.json %s", *pckg.Version)

	// Update package.json file
	kvPackageMetadata := updateVersionInCdnjs(ctx, pckg, packageJSONPath)

	// Git add/commit the updated package.json to cdnjs/cdnjs
	packages.GitAdd(ctx, cdnjsPath, path.Join(pckg.LibraryPath(), "package.json"))
	commitMsg := fmt.Sprintf("Set %s package.json (%s)", *pckg.Name, *pckg.Version)
	packages.GitCommit(ctx, cdnjsPath, commitMsg)

	// Git add/commit the updated non-human-readable metadata to cdnjs/logs
	packages.GitAdd(ctx, logsPath, pckg.Log("update metadata: %s: %s", *pckg.Version, kvPackageMetadata))
	logsCommitMsg := fmt.Sprintf("Set %s package metadata (%s)", *pckg.Name, *pckg.Version)
	packages.GitCommit(ctx, logsPath, logsCommitMsg)
}
