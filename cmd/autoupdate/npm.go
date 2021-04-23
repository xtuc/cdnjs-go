package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"sort"

	"github.com/cdnjs/tools/git"
	"github.com/cdnjs/tools/npm"
	"github.com/cdnjs/tools/packages"
	"github.com/cdnjs/tools/util"
)

func updateNpm(ctx context.Context, pckg *packages.Package) ([]newVersionToCommit, []version) {
	var newVersionsToCommit []newVersionToCommit
	var allVersions []version

	existingVersionSet := pckg.Versions()
	util.Debugf(ctx, "existing npm versions: %v\n", existingVersionSet)

	npmVersions, _ := npm.GetVersions(ctx, *pckg.Autoupdate.Target)
	lastExistingVersionInNpm, allExistingInCDNJS := npm.GetMostRecentExistingVersion(ctx, existingVersionSet, npmVersions)

	// add all existing versions to all versions list
	for _, v := range allExistingInCDNJS {
		allVersions = append(allVersions, version(v))
	}

	if lastExistingVersionInNpm != nil {
		util.Debugf(ctx, "last existing version: %s\n", lastExistingVersionInNpm.Version)

		versionDiff := npmVersionDiff(npmVersions, existingVersionSet)
		sort.Sort(npm.ByTimeStamp(versionDiff))

		newNpmVersions := make([]npm.Version, 0)

		for i := len(versionDiff) - 1; i >= 0; i-- {
			v := versionDiff[i]
			if v.TimeStamp.After(lastExistingVersionInNpm.TimeStamp) {
				newNpmVersions = append(newNpmVersions, v)
			}
		}

		sort.Sort(sort.Reverse(npm.ByTimeStamp(npmVersions)))

		newVersionsToCommit = doUpdateNpm(ctx, pckg, newNpmVersions)
	} else {
		if len(existingVersionSet) > 0 {
			// all existing versions are not on npm anymore
			util.Debugf(ctx, "all existing versions not on npm: %s\n", *pckg.Name)
		}
		// Import all the versions since we have no current npm versions locally.
		// Limit the number of version to an arbitrary number to avoid publishing
		// too many outdated versions.
		sort.Sort(sort.Reverse(npm.ByTimeStamp(npmVersions)))

		if len(npmVersions) > util.ImportAllMaxVersions {
			npmVersions = npmVersions[len(npmVersions)-util.ImportAllMaxVersions:]
		}

		// Reverse the array to have the older versions first
		// It matters when we will commit the updates
		sort.Sort(sort.Reverse(npm.ByTimeStamp(npmVersions)))

		newVersionsToCommit = doUpdateNpm(ctx, pckg, npmVersions)
	}

	// add all new versions to list of all versions
	for _, v := range newVersionsToCommit {
		allVersions = append(allVersions, version(v))
	}

	return newVersionsToCommit, allVersions
}

func doUpdateNpm(ctx context.Context, pckg *packages.Package, versions []npm.Version) []newVersionToCommit {
	newVersionsToCommit := make([]newVersionToCommit, 0)

	if len(versions) == 0 {
		return newVersionsToCommit
	}

	for _, version := range versions {
		pckgpath := path.Join(pckg.LibraryPath(), version.Version)

		if _, err := os.Stat(pckgpath); !os.IsNotExist(err) {
			util.Debugf(ctx, "%s already exists; aborting", pckgpath)
			continue
		}

		if git.IsPathIgnored(ctx, util.GetCDNJSPath(), pckgpath) {
			util.Debugf(ctx, "%s is ignored by git; aborting\n", pckgpath)
			continue
		}

		tarballDir := npm.DownloadTar(ctx, version.Tarball)
		filesToCopy := pckg.NpmFilesFrom(tarballDir)

		if len(filesToCopy) > 0 {
			util.Check(os.MkdirAll(pckgpath, os.ModePerm))
			for _, fileMoveOp := range filesToCopy {
				absFrom := path.Join(tarballDir, fileMoveOp.From)
				absDest := path.Join(pckgpath, fileMoveOp.To)

				if _, err := os.Stat(path.Dir(absDest)); os.IsNotExist(err) {
					util.Check(os.MkdirAll(path.Dir(absDest), os.ModePerm))
				}

				util.Debugf(ctx, "%s -> %s\n", absFrom, absDest)

				err := util.MoveFile(
					ctx,
					absFrom,
					absDest,
				)
				if err != nil {
					fmt.Println("could not move file:", err)
				}
			}

			newVersionsToCommit = append(newVersionsToCommit, newVersionToCommit{
				versionPath: pckgpath,
				newVersion:  version.Version,
				pckg:        pckg,
				timestamp:   version.TimeStamp,
			})
		} else {
			util.Debugf(ctx, "no files matched")
		}

		// clean up temporary tarball dir
		util.Check(os.RemoveAll(tarballDir))
	}

	return newVersionsToCommit
}

func npmVersionDiff(a []npm.Version, b []string) []npm.Version {
	diff := make([]npm.Version, 0)
	m := make(map[string]bool)

	for _, item := range b {
		m[item] = true
	}

	for _, item := range a {
		if _, ok := m[item.Version]; !ok {
			diff = append(diff, item)
		}
	}

	return diff
}
