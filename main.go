package main

import (
	"database/sql"
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
)

func main() {
	if len(os.Args) != 4 {
		fmt.Println("Usage:", os.Args[0], "path/to/domains path/to/rails/config/database.yml path/to/GeoLite2-City.mmdb")
		os.Exit(1)
	}

	dnsClient.ReadTimeout = timeout

	environment := os.Getenv("RAILS_ENV")
	if environment == "" {
		environment = "development"
	}

	if err := readDomains(os.Args[1]); err != nil {
		fmt.Println("unable to read domain list")
		panic(err)
	}

	// load database configuration
	connection = databasePath(os.Args[2], environment)

	// load path to GeoDB
	geoDbPath = os.Args[3]

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
