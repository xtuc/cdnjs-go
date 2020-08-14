package main

import (
	"flag"
	"fmt"

	"github.com/cdnjs/tools/kv"
	"github.com/cdnjs/tools/sentry"

	"github.com/cdnjs/tools/util"
)

var (
	// initialize standard debug logger
	logger = util.GetStandardLogger()
)

func init() {
	sentry.Init()
}

func main() {
	defer sentry.PanicHandler()
	var metaOnly bool
	flag.BoolVar(&metaOnly, "meta-only", false, "If set, only version metadata is uploaded to KV (no files).")
	flag.Parse()

	if util.IsDebug() {
		fmt.Println("Running in debug mode")
	}

	switch subcommand := flag.Arg(0); subcommand {
	case "upload":
		{
			pckgs := flag.Args()[1:]
			if len(pckgs) == 0 {
				panic("no packages specified")
			}

			kv.InsertFromDisk(logger, pckgs, metaOnly)
		}
	case "upload-aggregate":
		{
			pckgs := flag.Args()[1:]
			if len(pckgs) == 0 {
				panic("no packages specified")
			}

			kv.InsertAggregateMetadataFromScratch(logger, pckgs)
		}
	case "aggregate-packages":
		{
			kv.OutputAllAggregatePackages()
		}
	case "packages":
		{
			kv.OutputAllPackages()
		}
	case "files":
		{
			pckg := flag.Arg(1)
			if pckg == "" {
				panic("no package specified")
			}

			kv.OutputAllFiles(logger, pckg)
		}
	case "meta":
		{
			pckg := flag.Arg(1)
			if pckg == "" {
				panic("no package specified")
			}

			kv.OutputAllMeta(logger, pckg)
		}
	case "aggregate":
		{
			pckg := flag.Arg(1)
			if pckg == "" {
				panic("no package specified")
			}

			kv.OutputAggregate(pckg)
		}
	case "purge":
		{
			tag := flag.Arg(1)
			if tag == "" {
				panic("no tag specified")
			}

			util.Check(kv.PurgeTags([]string{tag}))
			fmt.Printf("Purged `%s`.\n", tag)
		}
	default:
		panic(fmt.Sprintf("unknown subcommand: `%s`", subcommand))
	}
}
