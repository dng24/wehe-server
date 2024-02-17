// Performs reverse geocode.
// Inspired by https://github.com/richardpenman/reverse_geocode/ (basically a GO version of this
// library)
// Uses data from all cities >1000 people population from geonames.org and a K-d tree to
// efficiently find the closest city to a given (latitude, longitude) coordinate in log n time.
package geolocation

import (
    "encoding/csv"
    "encoding/json"
    "math"
    "os"
    "strconv"

    "gonum.org/v1/gonum/spatial/kdtree"
)

const (
    geoDBPath = "res/geolocation/geoData.csv"
    countryMappingPath = "res/geolocation/countryMapping.json"
)

var tree *kdtree.Tree // the tree that allows us to find the closest city efficiently

type Location struct {
    Latitude float64 // latitude of the location
    Longitude float64 // longitude of the location
    City string // city name of the location
    Country string // country name of the location
    TimeZone string // IANA name of the time zone location is located in
}

// Initializes the K-d tree with all the city locations from the data file. This should be run only
// once.
// Returns any errors
func Init() error {
    locations, err := getLocations()
    if err != nil {
        return err
    }

    tree = kdtree.New(locations, false)
    return nil
}

// Gets the city information from the data file.
// Returns a list of locations or any errors
func getLocations() (locations, error) {
    // open and read in the JSON country code to country name map
    countryMappingFile, err := os.Open(countryMappingPath)
    if err != nil {
        return nil, err
    }
    defer countryMappingFile.Close()

    var countryMappingData map[string]string
    err = json.NewDecoder(countryMappingFile).Decode(&countryMappingData)
    if err != nil {
        return nil, err
    }

    // open and read in the city data from the csv
    geoDBFile, err := os.Open(geoDBPath)
    if err != nil {
        return nil, err
    }
    defer geoDBFile.Close()

    var locations locations
    reader := csv.NewReader(geoDBFile)
    // loop through each city
    for {
        locationSlice, err := reader.Read()
        if err != nil {
            if err.Error() == "EOF" {
                break
            }
            return nil, err
        }
        lat, err := strconv.ParseFloat(locationSlice[0], 64)
        if err != nil {
            return nil, err
        }
        long, err := strconv.ParseFloat(locationSlice[1], 64)
        if err != nil {
            return nil, err
        }
        location := Location{
            Latitude: lat,
            Longitude: long,
            City: locationSlice[4],
            Country: countryMappingData[locationSlice[3]], // convert 2 letter country code to country name
            TimeZone: locationSlice[2],
        }
        locations = append(locations, location)
    }
    return locations, nil
}

// Get the nearest city given a latitude and longitude.
// Returns the nearest city or any errors
func ReverseGeocode(latitude float64, longitude float64) (Location, error) {
    query := Location{
        Latitude: latitude,
        Longitude: longitude,
    }
    var keeper kdtree.Keeper
    keeper = kdtree.NewNKeeper(1) // tells the tree that we want nearest city
    tree.NearestSet(keeper, query) // do the query
    closestLocation := keeper.(*kdtree.NKeeper).Heap[0].Comparable.(Location) // get the result
    return closestLocation, nil
}

// Gets the distance between a dimension of two points in the tree. Satisfies the kdtree.Comparable
// interface.
// Returns distance
func (loc Location) Compare(c kdtree.Comparable, dimension kdtree.Dim) float64 {
    otherLoc := c.(Location)
    switch dimension {
    case 0: // dim 0 is latitude
        return loc.Latitude - otherLoc.Latitude
    case 1: // dim 1 is longitude
        return loc.Longitude - otherLoc.Longitude
    default:
        panic("Illegal dimension")
    }
}

// Gets number of dimensions in tree. Satisfies the kdtree.Comparable interface.
// Returns the number of dimensions
func (loc Location) Dims() int {
    return 2
}

// Calculates the Euclidean distance between two points. Satisfies the kdtree.Comparable interface.
// d = sqrt((a_lat - b_lat)^2 + (a_long - b_long)^2)
// Returns distance between two points
func (loc Location) Distance(c kdtree.Comparable) float64 {
    otherLoc := c.(Location)
    latDistSquared := math.Pow(loc.Latitude - otherLoc.Latitude, 2.0)
    longDistSquared := math.Pow(loc.Longitude - otherLoc.Longitude, 2.0)
    dist := math.Sqrt(latDistSquared + longDistSquared)
    return dist
}

// used for the kdtree.Interface
type locations []Location

// Returns an item at given index in slice of Location. Satisfies the kdtree.Interface interface.
func (locs locations) Index(i int) kdtree.Comparable {
    return locs[i]
}

// Returns the length of the slice of Location. Satisfies the kdtree.Interface interface.
func (locs locations) Len() int {
    return len(locs)
}

// Returns the pivot element of a given dimension to construct the tree. Satisfies the
// kdtree.Interface interface.
func (locs locations) Pivot(dimension kdtree.Dim) int {
    return plane{locations: locs, Dim: dimension}.Pivot()
}

// Returns a portion of the Location slice given the start and end index. Satisfies the
// kdtree.Interface interface.
func (locs locations) Slice(start int, end int) kdtree.Interface {
    return locs[start:end]
}

// used to help find pivot elements
type plane struct {
    kdtree.Dim
    locations
}

// Given two points, return whether the first point is less than the second point at the given
// dimension.
func (pln plane) Less(i int, j int) bool {
    switch pln.Dim {
    case 0: // 0 is latitude
        return pln.locations[i].Latitude < pln.locations[j].Latitude
    case 1: // 1 id longitude
        return pln.locations[i].Longitude < pln.locations[j].Longitude
    default:
        panic("Illegal dimension")
    }
}

// Finds the pivot element to construct the tree.
func (pln plane) Pivot() int {
    return kdtree.Partition(pln, kdtree.MedianOfMedians(pln))
}

// Given a start and end index, return a portion of the slice of Location.
func (pln plane) Slice(start int, end int) kdtree.SortSlicer {
    pln.locations = pln.locations[start:end]
    return pln
}

// Swap two Location elements.
func (pln plane) Swap(i int, j int) {
    pln.locations[i], pln.locations[j] = pln.locations[j], pln.locations[i]
}
