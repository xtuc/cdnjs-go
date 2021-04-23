package git

import (
	"context"
	"time"

	"github.com/cdnjs/tools/util"
)

// ByTimeStamp implements the sort.Interface for []Version,
// ordering from most recent to least recent time stamps.
type ByTimeStamp []Version

func (a ByTimeStamp) Len() int      { return len(a) }
func (a ByTimeStamp) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByTimeStamp) Less(i, j int) bool {
	return a[i].TimeStamp.After(a[j].TimeStamp)
}

// GetMostRecentExistingVersion gets the most recent git.Version based on time stamp
// that is currently downloaded as well as all existing versions in git.Version form.
func GetMostRecentExistingVersion(ctx context.Context, existingVersions []string, gitVersions []Version) (*Version, []Version) {
	// create map for fast lookups
	gitMap := make(map[string]Version)
	for _, v := range gitVersions {
		gitMap[v.Version] = v
	}

	// All existing, whether in git or not.
	var allExisting []Version

	// find most recent version
	var mostRecent *Version
	for _, existingVersion := range existingVersions {
		if version, ok := gitMap[existingVersion]; ok {
			allExisting = append(allExisting, version)
			if mostRecent == nil || version.TimeStamp.After(mostRecent.TimeStamp) {
				mostRecent = &version // new most recent found
			}
		} else {
			util.Debugf(ctx, "existing version not found on git: %s", existingVersion)
			allExisting = append(allExisting, Version{
				Version:   existingVersion,
				TimeStamp: time.Time{},
			})
		}
	}

	return mostRecent, allExisting
}

// GetMostRecentVersion gets the latest version in git based on time stamp.
func GetMostRecentVersion(gitVersions []Version) *Version {
	var mostRecent *Version

	for i := 0; i < len(gitVersions); i++ {
		if mostRecent == nil || gitVersions[i].TimeStamp.After(mostRecent.TimeStamp) {
			mostRecent = &gitVersions[i]
		}
	}

	return mostRecent
}
