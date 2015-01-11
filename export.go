package main

import (
	"database/sql"
	"encoding/csv"
	"os"
	"strings"
)

// Inspired by http://stackoverflow.com/a/14500756
// country_id will NOT be escaped for SQL!
func exportCsv(country_id string) {
	writer := csv.NewWriter(os.Stdout)
	columns := []string{"id", "ip", "name", "country_id", "city", "version", "checked_at"}
	columnCount := 7
	writer.Write(columns)

	// Open SQL connection
	db, err := sql.Open("mysql", connection)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	currentId := "0"
	batchSize := 1000

	// Build the SQL query
	query := "SELECT " + strings.Join(columns, ",") + " FROM nameservers WHERE state='valid' AND id > ?"
	if country_id != "all" {
		query += " AND country_id='" + country_id + "'"
	}
	query += " LIMIT ?"

	// Read in batches
	for {
		found := 0
		// Read the next batch
		rows, err := db.Query(query, currentId, batchSize)
		if err != nil {
			panic(err)
		}

		// Result is your slice string.
		rawResult := make([][]byte, columnCount)
		result := make([]string, columnCount)

		dest := make([]interface{}, columnCount) // A temporary interface{} slice
		for i, _ := range rawResult {
			dest[i] = &rawResult[i] // Put pointers to each string in the interface slice
		}

		for rows.Next() {
			if err = rows.Scan(dest...); err != nil {
				panic(err)
			}

			for i, raw := range rawResult {
				if raw == nil {
					result[i] = ""
				} else {
					result[i] = string(raw)
				}
			}

			currentId = result[0]
			found++

			writer.Write(result)
		}

		// Last batch?
		if found < batchSize {
			break
		}
	}

	writer.Flush()
}
