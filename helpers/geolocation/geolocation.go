// Downloads and processes data files to do reverse geocoding.
package geolocation

import (
    "archive/zip"
    "bufio"
    "encoding/csv"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "path/filepath"
    "strings"
)

const (
    outputDir = "../res/geolocation/"

    geoZipFileName = "cities1000.zip"
    geoDataURL = "https://download.geonames.org/export/dump/" + geoZipFileName
    geoZipOutputDir = "cities1000"
    geoDataFile = "cities1000/cities1000.txt"
    geoDataOutput = outputDir + "geoData.csv"

    countryCodeFileName = "countryInfo.txt"
    countryCodeURL = "https://download.geonames.org/export/dump/" + countryCodeFileName
    countryCodeOutput = outputDir + "countryMapping.json"
)

// Downloads raw data, processes the data, and outputs to files on disk.
// Returns any errors
func GetGeolocationData() error {
    // ensure output directory exists
    err := os.MkdirAll(outputDir, os.ModePerm)
    if err != nil {
        return err
    }

    // processes the data containing geo data
    err = getGeoData()
    if err != nil {
        return err
    }

    // processes the country code to country name mapping
    err = getCountryCodeMapping()
    if err != nil {
        return err
    }
    return nil
}

// Downloads raw geo data about every city in the world >1000 people population from geonames.org and
// processes it into an output csv.
// Returns any errors
func getGeoData() error {
    // Download the zip file
    err := downloadFile(geoZipFileName, geoDataURL)
    if err != nil {
        return err
    }

    // Unzip the downloaded file, which contains a directory containing a txt file with the data
    err = unzip(geoZipFileName, geoZipOutputDir)
    if err != nil {
        return err
    }

    // Take raw data and process it into csv for Wehe reverse geocoding
    err = processGeoNamesData()
    if err != nil {
        return err
    }
    return nil
}

// Retrieves the latitude, longitude, IANA time zone, 2 letter country code, and city name of each
// city in the world with >1000 people population. Writes these fields to a CSV.
// Returns any errors
func processGeoNamesData() error {
    inFile, err := os.Open(geoDataFile)
    if err != nil {
        return err
    }
    defer inFile.Close()

    geoData := [][]string{}

    // Create a scanner to read the file line by line
    scanner := bufio.NewScanner(inFile)

    // Iterate over each line
    for scanner.Scan() {
        // raw file is tab delimited
        fields := strings.Split(scanner.Text(), "\t")
        // retrieve the relavent fields from the raw file
        // latitude, longitude, IANA time zone, 2 letter country code, city name
        location := []string{fields[4], fields[5], fields[17], fields[8], fields[1]}
        geoData = append(geoData, location)
    }

    // Create a new CSV file
    outFile, err := os.Create(geoDataOutput)
    if err != nil {
        panic(err)
    }
    defer outFile.Close()

    writer := csv.NewWriter(outFile)
    defer writer.Flush()

    // write csv to file
    for _, row := range geoData {
        err := writer.Write(row)
        if err != nil {
            return err
        }
    }

    // remove zip file and unzipped folder
    err = os.Remove(geoZipFileName)
    if err != nil {
        fmt.Println(err)
    }
    err = os.RemoveAll(geoZipOutputDir)
    if err != nil {
        fmt.Println(err)
    }
    return nil
}

// Downloads country information from geonames.org. Extracts the 2 letter country code and country
// and country name from the data and writes extracted information to json file.
// Returns any errors
func getCountryCodeMapping() error {
    // download the txt file
    err := downloadFile(countryCodeFileName, countryCodeURL)
    if err != nil {
        return err
    }

    inFile, err := os.Open(countryCodeFileName)
    if err != nil {
        return err
    }
    defer inFile.Close()

    countryCodeMapping := make(map[string]string)

    // loop over each line of the file ignoring lines starting with #
    scanner := bufio.NewScanner(inFile)
    for scanner.Scan() {
        line := scanner.Text()
        if line[0] != '#' {
            fields := strings.Split(line, "\t")
            // extract 2 letter country code (fields[0]) and country name (fields[4])
            countryCodeMapping[fields[0]] = fields[4]
        }
    }

    jsonData, err := json.MarshalIndent(countryCodeMapping, "", "  ")
    if err != nil {
        return err
    }
    outFile, err := os.Create(countryCodeOutput)
    if err != nil {
        return err
    }
    defer outFile.Close()

    _, err = outFile.Write(jsonData)
    if err != nil {
        return err
    }

    err = os.Remove(countryCodeFileName)
    if err != nil {
        fmt.Println(err)
    }
    return nil
}

// Downloads a file from a URL.
// filepath: file path where file should be stored locally
// url: the URL to download file from
// Returns any errors
func downloadFile(filepath string, url string) error {
    // Create the file
    out, err := os.Create(filepath)
    if err != nil {
        return err
    }
    defer out.Close()

    // Get the data
    resp, err := http.Get(url)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    // Write the body to file
    _, err = io.Copy(out, resp.Body)
    if err != nil {
        return err
    }

    return nil
}

// Unzips a zip file.
// src: the path to the zip file
// dest: where the contents of the unzipped file should be placed
// Returns any errors
func unzip(src string, dest string) error {
    // Create a directory to extract the zip file into
    err := os.MkdirAll(dest, os.ModePerm)
    if err != nil {
        return err
    }

    // Open the zip file
    r, err := zip.OpenReader(src)
    if err != nil {
        return err
    }
    defer r.Close()

    // Extract each file from the zip archive
    for _, f := range r.File {
        rc, err := f.Open()
        if err != nil {
            return err
        }
        defer rc.Close()

        // Create the file
        path := filepath.Join(dest, f.Name)
        if f.FileInfo().IsDir() {
            os.MkdirAll(path, os.ModePerm)
        } else {
            file, err := os.Create(path)
            if err != nil {
                return err
            }
            defer file.Close()

            // Write the file contents
            _, err = io.Copy(file, rc)
            if err != nil {
                return err
            }
        }
    }

    return nil
}
