package main

import (
	"context"
	"fmt"
	"os"
	"path"
	"sort"

	"github.com/cdnjs/tools/git"
	"github.com/cdnjs/tools/packages"
	"github.com/cdnjs/tools/util"
)

var (
	gitCache = path.Join(basePath, "git-cache")
)

func isValidGit(ctx context.Context, pckgdir string) bool {
	_, err := os.Stat(path.Join(pckgdir, ".git"))
	return !os.IsNotExist(err)
}

func updateGit(ctx context.Context, pckg *packages.Package) ([]newVersionToCommit, []version) {
	var newVersionsToCommit []newVersionToCommit
	var allVersions []version

	packageGitcache := path.Join(gitCache, *pckg.Name)
	// If the local copy of the package's git doesn't exists, Clone it. If it does
	// just fetch new tags
	if _, err := os.Stat(packageGitcache); os.IsNotExist(err) {
		util.Check(os.MkdirAll(packageGitcache, os.ModePerm))

		out, err := git.Clone(ctx, *pckg.Autoupdate.Target, packageGitcache)
		if err != nil {
			util.Errf(ctx, "could not clone repo: %s: %s\n", err, out)
			return newVersionsToCommit, nil
		}
	} else {
		if isValidGit(ctx, packageGitcache) {
			out, fetcherr := git.Fetch(ctx, packageGitcache)
			if fetcherr != nil {
				util.Errf(ctx, "could not fetch repo %s: %s\n", fetcherr, out)
				return newVersionsToCommit, nil
			}
		} else {
			util.Errf(ctx, "invalid git repo\n")
			return newVersionsToCommit, nil
		}
	}

	gitVersions, _ := git.GetVersions(ctx, packageGitcache)
	existingVersionSet := pckg.Versions()

	util.Debugf(ctx, "existing git versions: %v\n", existingVersionSet)
	lastExistingVersionInGit, allExistingInCDNJS := git.GetMostRecentExistingVersion(ctx, existingVersionSet, gitVersions)

	// add all existing versions to all versions list
	for _, v := range allExistingInCDNJS {
		allVersions = append(allVersions, version(v))
	}

	if lastExistingVersionInGit != nil {
		util.Debugf(ctx, "last existing version: %s\n", lastExistingVersionInGit.Version)

		versionDiff := gitVersionDiff(gitVersions, existingVersionSet)

		newGitVersions := make([]git.Version, 0)

		for i := len(versionDiff) - 1; i >= 0; i-- {
			v := versionDiff[i]
			if v.TimeStamp.After(lastExistingVersionInGit.TimeStamp) {
				newGitVersions = append(newGitVersions, v)
			}
		}

		util.Debugf(ctx, "new versions: %s\n", newGitVersions)

		sort.Sort(sort.Reverse(git.ByTimeStamp(newGitVersions)))

		newVersionsToCommit = doUpdateGit(ctx, pckg, packageGitcache, newGitVersions)
	} else {
		if len(existingVersionSet) > 0 {
			// all existing versions are not on git anymore
			util.Debugf(ctx, "all existing versions not on git: %s\n", *pckg.Name)
		}
		// Import all the versions since we have none locally.
		// Limit the number of version to an arbitrary number to avoid publishing
		// too many outdated versions.
		sort.Sort(sort.Reverse(git.ByTimeStamp(gitVersions)))

		if len(gitVersions) > util.ImportAllMaxVersions {
			gitVersions = gitVersions[len(gitVersions)-util.ImportAllMaxVersions:]
		}

		// Reverse the array to have the older versions first
		// It matters when we will commit the updates
		sort.Sort(sort.Reverse(git.ByTimeStamp(gitVersions)))

		newVersionsToCommit = doUpdateGit(ctx, pckg, packageGitcache, gitVersions)
	}

	// add all new versions to list of all versions
	for _, v := range newVersionsToCommit {
		allVersions = append(allVersions, version(v))
	}

	return newVersionsToCommit, allVersions
}

func doUpdateGit(ctx context.Context, pckg *packages.Package, gitpath string, versions []git.Version) []newVersionToCommit {
	newVersionsToCommit := make([]newVersionToCommit, 0)

	if len(versions) == 0 {
		return newVersionsToCommit
	}

	for _, gitversion := range versions {
		git.ForceCheckout(ctx, gitpath, gitversion.Tag)
		filesToCopy := pckg.NpmFilesFrom(gitpath)

		pckgpath := path.Join(pckg.LibraryPath(), gitversion.Version)

		if _, err := os.Stat(pckgpath); !os.IsNotExist(err) {
			util.Debugf(ctx, "%s already exists; aborting\n", pckgpath)
			continue
		}

		if git.IsPathIgnored(ctx, util.GetCDNJSPath(), pckgpath) {
			util.Debugf(ctx, "%s is ignored by git; aborting\n", pckgpath)
			continue
		}

		if len(filesToCopy) > 0 {
			util.Check(os.MkdirAll(pckgpath, os.ModePerm))
			for _, fileMoveOp := range filesToCopy {
				absFrom := path.Join(gitpath, fileMoveOp.From)
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
				newVersion:  gitversion.Version,
				pckg:        pckg,
				timestamp:   gitversion.TimeStamp,
			})
		} else {
			util.Debugf(ctx, "no files matched\n")
		}
	}

	return newVersionsToCommit
}

func gitVersionDiff(a []git.Version, b []string) []git.Version {
	diff := make([]git.Version, 0)
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
