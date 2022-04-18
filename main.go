package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

const migrationVersion int = 1

var (
	ErrCurrentVersionStateCorrupted = errors.New("version state file corrupted")
	ErrCannotRetrieveCurrentVersion = errors.New("cannot access version state file")
	versionFilePath                 = "version.state"
	fullUpdate                      = -1
)

// const migrationsDirectory = "migrations/"
var migrationsDirectory = "migrations/"

func main() {
	var generate, up, down, state, time, init bool
	var targetVersion int
	flags := flag.NewFlagSet("main", flag.ExitOnError)
	flags.BoolVar(&generate, "generate", false, "generate a new migration")
	flags.BoolVar(&up, "up", false, "upgrade to version -target or to last version if no target is supplied")
	flags.BoolVar(&down, "down", false, "downgrade to version -target or by one version if no target is supplied")
	flags.BoolVar(&state, "state", false, "display current version")
	flags.BoolVar(&time, "time", false, "preprend outputs with date time")
	flags.IntVar(&targetVersion, "target", fullUpdate, "target version")
	flags.BoolVar(&init, "init", false, "init the migration version control system")
	if err := flags.Parse(os.Args[1:]); err != nil {
		fmt.Println(err)
		os.Exit(2)
	}
	logger := log.New(os.Stdout, "", 0)
	if time {
		logger.SetFlags(logger.Flags() + log.Lmicroseconds)
	}

	if generate {
		generateNewMigration()
		return
	}

	if init {
		err := ioutil.WriteFile(versionFilePath, []byte("0"), fs.ModePerm)
		if err != nil {
			fmt.Println("failed to initialiaze migration version control system")
			fmt.Println(err)
			return
		}
		fmt.Println("migration version control system initialized")
		return
	}

	if up {
		upgrade(targetVersion, logger)
		return
	}

	if down {
		downgrade(targetVersion, logger)
		return
	}

	if state {
		version, err := detectVersion()
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}
		fmt.Printf("%d", version)
		return
	}

	flag.Usage()
}

func detectVersion() (int, error) {
	data, err := ioutil.ReadFile(versionFilePath)
	if os.IsNotExist(err) {
		return 0, ErrCannotRetrieveCurrentVersion
	}

	if err != nil {
		return 0, err
	}

	var versionData string
	versionData = fmt.Sprintf("%s", data)
	versionData = strings.Trim(versionData, "")
	var d int
	parsed, err := fmt.Sscanf(versionData, "%d", &d)
	if err != nil {
		return 0, err
	}

	if parsed == 0 {
		return 0, ErrCurrentVersionStateCorrupted
	}

	// // versionData = strings.Trim(versionData, "\0")
	// version, err := strconv.ParseInt(d, 10, 8)
	// if err != nil {
	// 	panic(err)
	// }

	return d, nil
}

func downgrade(targetVersion int, logger *log.Logger) {
	currentVersion, err := detectVersion()
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	if targetVersion > currentVersion {
		logger.Printf("Current version(%d) is already behind target version(%d)\n", currentVersion, targetVersion)
		os.Exit(1)
	}

	if targetVersion == fullUpdate {
		logger.Println("Downgrading to last known version")
	}

	versions, err := listMigrations()
	if err != nil {
		panic(err)
	}

	sort.Sort(sort.Reverse(sort.IntSlice(versions)))
	for _, v := range versions {
		if v < targetVersion && targetVersion != fullUpdate {
			logger.Printf("Stopping downgrade since target version(%d) is ahead of migration version(%d)", targetVersion, v)
			break
		}
		if currentVersion <= v {
			logger.Printf("Ignoring migration %d because we're already below behind it(%d)\n", v, currentVersion)
			continue
		}

		logger.Printf("Downgrade to %d\n", v)
		success := runMigration(v, logger, "-down")
		if !success {
			logger.Printf("Migration %d failed", v)
			return
		}
		err := ioutil.WriteFile(versionFilePath, []byte(fmt.Sprintf("%d", v)), fs.ModeExclusive)
		if err != nil {
			logger.Println("Failed to save current version to state file : ")
			logger.Println(err)
			os.Exit(1)
		}
		logger.Printf("Current version saved to state file(%d)", v)
	}

	err = ioutil.WriteFile(versionFilePath, []byte("0"), fs.ModeExclusive)
	if err != nil {
		fmt.Println("Failed to save applied version")
		fmt.Println(err)
		os.Exit(1)
	}

	return
}

func upgrade(targetVersion int, logger *log.Logger) {
	currentVersion, err := detectVersion()
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}

	if targetVersion < currentVersion && targetVersion != fullUpdate {
		logger.Printf("Current version(%d) is already ahead of target version(%d)\n", currentVersion, targetVersion)
		os.Exit(1)
	}

	if targetVersion == fullUpdate {
		logger.Println("Upgrading to last known version")
	}

	versions, err := listMigrations()
	if err != nil {
		panic(err)
	}

	sort.Ints(versions)

	for _, v := range versions {
		if v > currentVersion && targetVersion != fullUpdate {
			logger.Printf("Stopping upgrade since target version(%d) is below migration version(%d)", targetVersion, v)
			break
		}

		if currentVersion >= v {
			logger.Printf("Ignoring migration %d because we're already ahead of it(%d)\n", v, currentVersion)
			continue
		}

		logger.Printf("Upgrading to %d\n", v)
		success := runMigration(v, logger, "-up")
		if !success {
			logger.Printf("Migration %d failed", v)
			return
		}
		err := ioutil.WriteFile(versionFilePath, []byte(fmt.Sprintf("%d", v)), fs.ModeExclusive)
		if err != nil {
			logger.Println("Failed to save current version to state file : ")
			logger.Println(err)
			os.Exit(1)
		}
		logger.Printf("Current version saved to state file(%d)", v)
	}
	return
}

func listMigrations() ([]int, error) {
	var versions []int
	files, err := ioutil.ReadDir(migrationsDirectory)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		cmd := exec.Command("go", "run", fmt.Sprintf("%s%s", migrationsDirectory, file.Name()), "-v")

		output, err := cmd.CombinedOutput()
		if err != nil {
			return nil, err
		}

		var version int
		_, err = fmt.Sscanf(string(output), "%d", &version)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}

	return versions, nil
}

// runMigration run a specific migration and return true if migration is sucessful
func runMigration(v int, logger *log.Logger, action string) bool {
	start := time.Now()
	cmd := exec.Command("go", "run", fmt.Sprintf("%s%d.go", migrationsDirectory, v), action)
	end := time.Now()

	output, err := cmd.CombinedOutput()
	if err != nil {
		panic(err)
	}
	exitCode := cmd.ProcessState.ExitCode()

	flags := logger.Flags()
	logger.Printf("Took %s\n", end.Sub(start))
	logger.Println("Migration output :")
	logger.Println(strings.Repeat("=", 120))
	logger.SetFlags(0)
	logger.Print(string(output))
	logger.SetFlags(flags)
	logger.Println(strings.Repeat("=", 120))

	return exitCode == 0
}
