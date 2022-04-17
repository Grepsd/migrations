package main

import (
	"fmt"
	"log"
	"os"
	"text/template"
	"time"
)

var migrationTemplate = `
package main

import (
	"flag"
	"fmt"
  "os"
)

const migrationVersion int = {{ .CreationDatetime }}

func main() {
	version := flag.Bool("v", false, "display current version")
	up := flag.Bool("up", false, "run upgrade")
	down := flag.Bool("down", false, "run downgrade")
	flag.Parse()

	if *version {
		fmt.Println(migrationVersion)
		return
	}

	if *up {
		Up()
    os.Exit(0)
		return
	}

	if *down {
		Down()
    os.Exit(0)
		return
	}

	flag.Usage()
}

func Up() {
  fmt.Printf("upgrading to %d\n", migrationVersion)
}

func Down() {
  fmt.Printf("downgrading to %d\n", migrationVersion)
}
`

func generateNewMigration() {
	tpl, err := template.New("migration").Parse(migrationTemplate)
	if err != nil {
		panic(err)
	}

	migrationData := struct {
		CreationDatetime string
	}{
		fmt.Sprint(time.Now().Unix()),
	}

	if err := os.Mkdir(migrationsDirectory, 0o744); err != nil && !os.IsExist(err) {
		panic(err)
	}

	filePath := fmt.Sprintf("%s%s.go", migrationsDirectory, migrationData.CreationDatetime)
	fd, err := os.Create(filePath)
	if err != nil {
		panic(err)
	}
	defer fd.Close()

	err = tpl.Execute(fd, migrationData)
	if err != nil {
		panic(err)
	}

	logger := log.New(os.Stdout, "", log.Ltime|log.Ldate)
	logger.Printf("New migration generated in '%s'\n", filePath)
}
