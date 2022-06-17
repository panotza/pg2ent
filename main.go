package main

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"

	"github.com/panotza/pg2ent/postgres"
	"github.com/panotza/pg2ent/scepter"
	"gopkg.in/yaml.v3"
)

func main() {
	cf, err := os.ReadFile("pg2ent.yaml")
	if err != nil {
		log.Fatalf("read config file error %v", err)
	}
	var c scepter.Config
	err = yaml.Unmarshal(cf, &c)
	if err != nil {
		log.Fatalf("error: %v", err)
	}

	if c.SQLFile == "" {
		log.Fatal("sql file is required")
	}
	if c.OutDir == "" {
		log.Fatal("output directory is required")
	}

	sql, err := ioutil.ReadFile(c.SQLFile)
	if err != nil {
		log.Fatalf("read file error %v", err)
	}

	tables := postgres.ParseSQL(string(sql))
	log.Printf("found %d tables", len(tables))

	err = os.Mkdir(c.OutDir, 0755)
	if err != nil {
		if !os.IsExist(err) {
			log.Fatalf("create folder failed %v", err)
		}
	}

	st := scepter.NewScepter(c)
	for _, t := range tables {
		f, err := os.Create(filepath.Join(c.OutDir, scepter.Singularize(t.Name)+".go"))
		if err != nil {
			log.Fatalf("create file failed %v", err)
		}
		defer f.Close()

		if err := st.Generate(f, t); err != nil {
			log.Panic(err)
		}
	}
}
