// Provides different analyses that can be run on the tests.
package analysis

import (
    "fmt"
    "math/rand"
    "os/exec"
    "slices"
    "strings"
    "strconv"
    "time"

    "gonum.org/v1/gonum/stat"
)

const (
    alpha = 0.95
    r = 100.0
)

// An object that holds the results of different statistical analyses.
type AnalysisResults struct {
    ThroughputStats *DataSetStats
    SampleTimesStats *DataSetStats
    Area float64
    XPutMin float64
    Area0var float64
    KS2dVal float64
    KS2pVal float64
    DValAvg float64
    PValAvg float64
    KS2AcceptRatio float64
}

func NewAnalysisResults(throughputStats *DataSetStats, sampleTimesStats *DataSetStats,
    area float64, xPutMin float64, area0var float64, ks2dVal float64, ks2pVal float64,
    dValAvg float64, pValAvg float64, ks2AcceptRatio float64) *AnalysisResults {
    return &AnalysisResults{
        ThroughputStats: throughputStats,
        SampleTimesStats: sampleTimesStats,
        Area: area,
        XPutMin: xPutMin,
        Area0var: area0var,
        KS2dVal: ks2dVal,
        KS2pVal: ks2pVal,
        DValAvg: dValAvg,
        PValAvg: pValAvg,
        KS2AcceptRatio: ks2AcceptRatio,
    }
}

// An object containing the data itself and basic statistics.
type DataSetStats struct {
    Data []float64
    Max float64
    Min float64
    Average float64
    Median float64
    StandardDeviation float64
}

// Creates a new DataSetStats object. Removes any values in the data that are 0.
// data: the data to use for analyses. Data cannot be empty or contain all 0s.
// Returns a new DataSetStats object or any errors
func NewDataSetStats(data []float64) (*DataSetStats, error) {
    var cleanedData []float64
    for _, value := range data {
        if value != 0 {
            cleanedData = append(cleanedData, value)
        }
    }
    if len(cleanedData) == 0 {
        return nil, fmt.Errorf("Slice cannot be of length 0.")
    }

    sortedCleanedData := make([]float64, len(cleanedData))
    copy(sortedCleanedData, cleanedData)
    slices.Sort(sortedCleanedData)

    return &DataSetStats{
        Data: cleanedData,
        Max: sortedCleanedData[len(sortedCleanedData) - 1],
        Min: sortedCleanedData[0],
        Average: stat.Mean(sortedCleanedData, nil),
        Median: stat.Quantile(0.5, stat.Empirical, sortedCleanedData, nil),
        StandardDeviation: stat.PopStdDev(sortedCleanedData, nil),
    }, nil
}

// Gets the minimum value of the items in two slices.
// slice1: the first slice of data
// slice2: the second slice of data
// Returns the minimum value of the two slices
func CalculateMinValueOfTwoSlices(slice1 []float64, slice2 []float64) float64 {
    return slices.Min(append(slice1, slice2...))
}

func CalculateArea0Var(avg1 float64, avg2 float64) float64 {
    return (avg2 - avg1) / slices.Max([]float64{avg1, avg2})
}

// Performs a two-sample Kolmogorov-Smirnov test using Python's scipy library.
// data1: first sample of data, assumed to be drawn from a continuous distribution, can be
//     different size than data2
// data2: second sample of data, assumed to be drawn from a continuous distribution, can be
//     different size than data1
// Returns the KS test statistic and p-value, or any errors
func KS2Samp(data1 []float64, data2 []float64) (float64, float64, error) {
    data1Formatted := strings.ReplaceAll(fmt.Sprintf("%g", data1), " ", ",")
    data2Formatted := strings.ReplaceAll(fmt.Sprintf("%g", data2), " ", ",")
    ksTestCmd := fmt.Sprintf("from scipy.stats import ks_2samp; (stat,pval) = ks_2samp(%s,%s); print(stat,pval)",
        data1Formatted, data2Formatted)
    cmd := exec.Command("python3", "-c", ksTestCmd)
    var stdout strings.Builder
    var stderr strings.Builder
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr
    err := cmd.Run()
    if err != nil {
        return -1.0, -1.0, fmt.Errorf("Error running python KS analysis: %v\n%s", err, stderr.String())
    }

    result := strings.Split(stdout.String(), " ")
    if len(result) != 2 {
        return -1.0, -1.0, fmt.Errorf("Expected two space-delimited floats, got: %s", stdout.String())
    }

    statistic, err := strconv.ParseFloat(strings.TrimSpace(result[0]), 64)
    if err != nil {
        return -1.0, -1.0, fmt.Errorf("Statistic is not a float: %s", result[0])
    }

    pvalue, err := strconv.ParseFloat(strings.TrimSpace(result[1]), 64)
    if err != nil {
        return -1.0, -1.0, fmt.Errorf("P value is not a float: %s", result[1])
    }

    return statistic, pvalue, nil
}

// Taken from NetPolice paper:
//
// This function uses Jackknife, a commonly-used non-parametric re-sampling method, to verify the
// validity of the K-S test statistic. The idea is to randomly select half of the samples from the
// two original input sets and apply the K-S test on the two new subsets of samples. This process
// is repeated r times. If the results of over B% of the r new K-S tests are the same as that of
// the original test, we conclude that the original K-S test statistic is valid.
//
// data1: first sample of data, assumed to be drawn from a continuous distribution, can be
//     different size than data2
// data2: second sample of data, assumed to be drawn from a continuous distribution, can be
//     different size than data1
// ks2pVal: p-value of the first run of the two-sample KS test
// Returns average statistic, average p-value, and percentage of runs where p-value was accepted,
//     or any errors
func SampleKS2(data1 []float64, data2 []float64, ks2pVal float64) (float64, float64, float64, error) {
    greater := ks2pVal >= (1 - alpha)

    var dVals []float64
    var pVals []float64
    accept := 0.0
    for i := 0.0; i < r; i++ {
        sub1, err := randomSample(data1, len(data1) / 2)
        if err != nil {
            return -1.0, -1.0, -1.0, err
        }
        sub2, err := randomSample(data2, len(data2) / 2)
        if err != nil {
            return -1.0, -1.0, -1.0, err
        }
        dVal, pVal, err := KS2Samp(sub1, sub2)
        if err != nil {
            return -1.0, -1.0, -1.0, err
        }
        dVals = append(dVals, dVal)
        pVals = append(pVals, pVal)

        if greater {
            if pVal > (1 - alpha) {
                accept++
            }
        } else {
            if pVal < (1 - alpha) {
                accept++
            }
        }
    }

    dValAvg := stat.Mean(dVals, nil)
    pValAvg := stat.Mean(pVals, nil)
    return dValAvg, pValAvg, accept / r, nil
}

// Choose a random subset of given data.
// data: the data to choose random values from
// newSize: number of random samples to choose
// Returns a random subset of the given data, or any errors
func randomSample(data []float64, newSize int) ([]float64, error) {
    if newSize < 0 || newSize > len(data) {
        return nil, fmt.Errorf("Sample larger than population or is negative: %d", newSize)
    }
    if newSize == 0 {
        return []float64{}, nil
    }
    if newSize == len(data) {
        return data, nil
    }

    rand.Seed(time.Now().UnixNano())

    shuffled := make([]float64, len(data))
    copy(shuffled, data)
    rand.Shuffle(len(shuffled), func(i int, j int) {
        shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
    })

    return shuffled[:newSize], nil
}
