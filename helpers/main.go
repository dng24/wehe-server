// Main file for helper scripts.
package main

import (
    "fmt"

    "helpers/geolocation"
)

func main() {
    //TODO: reformat to allow different scripts to be run using command line args

    err := geolocation.GetGeolocationData()
    if err != nil {
        fmt.Println(err)
    }
}
