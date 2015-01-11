package main

import (
	"database/sql"
	"flag"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"os"
	"runtime"
)

const (
	// The nameserver that every other is compared with
	referenceNameserver = "8.8.8.8"

	// Timeout for DNS queries
	timeout = 3 * 1e9
)

var (
	pending      = make(chan *job, 100)
	finished     = make(chan *job, 100)
	done         = make(chan bool)
	workersLimit = 1
	connection   string
	domainArg    string
)

func main() {

	databaseArg := flag.String("database", "database.yml", "Path to file containing the database configuration")
	country := flag.String("country", "all", "country to export")
	flag.StringVar(&domainArg, "domains", "domains.txt", "Path to file containing the domain list")
	flag.StringVar(&geoDbPath, "geodb", "GeoLite2-City.mmdb", "Path to GeoDB database")
	flag.Parse()
	tail := flag.Args()

	// set the environment
	environment := os.Getenv("RAILS_ENV")
	if environment == "" {
		environment = "development"
	}

	// load database configuration
	connection = databasePath(*databaseArg, environment)

	if len(tail) == 0 {
		fmt.Println("No command given")
		os.Exit(1)
	}

	if tail[0] == "check" {
		mainCheck()
	} else if tail[0] == "csv" {
		exportCsv(*country)
	} else {
		fmt.Println("Unknown command: ", tail[0])
		os.Exit(1)
	}
}

func mainCheck() {
	dnsClient.ReadTimeout = timeout

	if err := readDomains(domainArg); err != nil {
		fmt.Println("unable to read domain list")
		panic(err)
	}

	// Use all cores
	cpus := runtime.NumCPU()
	runtime.GOMAXPROCS(cpus)
	workersLimit = 8 * cpus

	// Get results from the reference nameserver
	res, err := resolveDomains(referenceNameserver)
	if err != nil {
		panic(err)
	}
	expectedResults = res

	go resultWriter()

	// Start workers
	for i := 0; i < workersLimit; i++ {
		go worker()
	}

	createJobs()

	// wait for resultWriter to finish
	<-done
}

func createJobs() {
	currentId := 0
	batchSize := 1000

	// Open SQL connection
	db, err := sql.Open("mysql", connection)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	for {
		// Read the next batch
		rows, err := db.Query("SELECT id, ip FROM nameservers WHERE id > ? LIMIT ?", currentId, batchSize)
		if err != nil {
			panic(err)
		}

		found := 0
		for rows.Next() {
			j := new(job)

			// get RawBytes from data
			err = rows.Scan(&j.id, &j.address)
			if err != nil {
				panic(err)
			}
			pending <- j
			currentId = j.id
			found += 1
		}
		rows.Close()

		// Last batch?
		if found < batchSize {
			close(pending)
			return
		}
	}
}

func worker() {
	for {
		job := <-pending
		if job != nil {
			executeJob(job)
			finished <- job
		} else {
			log.Println("received all jobs")
			finished <- nil
			return
		}
	}
}

func resultWriter() {
	// Open SQL connection
	db, err := sql.Open("mysql", connection)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	stm, err := db.Prepare(
		"UPDATE nameservers SET name=?, state=?, error=?, version=?, checked_at=NOW(), country_id=?, city=?," +
			"state_changed_at = (CASE WHEN ? != state THEN NOW() ELSE state_changed_at END )" +
			"WHERE id=?")
	defer stm.Close()

	doneCount := 0
	for doneCount < workersLimit {
		res := <-finished
		// log.Println("finished job", res.id)
		if res == nil {
			doneCount++
			log.Println("worker terminated")
		} else {
			log.Println(res)
			stm.Exec(res.name, res.state, res.err, res.version, res.country, res.city, res.state, res.id)
		}
	}
	done <- true
}

// consumes a job and writes the result in the given job
func executeJob(job *job) {
	// log.Println("received job", job.id)

	// GeoDB lookup
	job.country, job.city = location(job.address)

	// Run the check
	err := check(job)
	job.name = ptrName(job.address)

	// query the bind version
	if err == nil || err.Error() != "i/o timeout" {
		job.version = version(job.address)
	}

	if err == nil {
		job.state = "valid"
		job.err = ""
	} else {
		job.state = "invalid"
		job.err = err.Error()
	}
}
