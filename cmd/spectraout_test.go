package cmd

import (
	"archive/zip"
	"fmt"
	"os"
	"strings"
	"testing"
)

const (
	testDataDir   = ".." + fpsep + "testdata"
	zipname       = testDataDir + fpsep + defaultZipName
	testJobFormat = testDataDir + fpsep + "CH4,x=1e-6,T=300K,P=1atm,L=100cm,simNum%d.csv"
	numberOfJobs  = 3
)

func TestPrettyFormat(t *testing.T) {
	tests := map[float64]string{
		0:          "0",
		2:          "2",
		999:        "999",
		999.999:    "999.999",
		999.9999:   "1e+03",
		1000:       "1e+03",
		1e99:       "1e+99",
		2.1:        "2.100",
		2.0001:     "2",
		0.35:       "0.350",
		0.0000432:  "4.320e-05",
		0.0009:     "9e-04",
		0.0009999:  "9.999e-04",
		0.00099999: "0.001",
	}
	for input, expected := range tests {
		output := prettyF(input)
		negOutput := prettyF(-input)
		if output != expected {
			t.Errorf("expected :%s\tgot: %s", expected, output)
		}
		if negOutput != "-"+expected && -input < 0 {
			t.Errorf("expected :%s\tgot: %s", "-"+expected, output)
		}
	}
}

func TestJoinSpectra(t *testing.T) {
	// Create a buffer to write our archive to.
	err := createSpectraZip()
	if err != nil {
		panic(err)
	}
	err = processSpectra(zipname, testDataDir)
	if err != nil {
		t.Error(err)
	}

}

func createSpectraZip() error {
	fo, err := os.Create(zipname)
	if err != nil {
		return err
	}
	defer fo.Close()
	// Create a new zip archive.
	w := zip.NewWriter(fo)
	var files []*os.File
	for i := 0; i < numberOfJobs; i++ {
		fi, err := os.Open(fmt.Sprintf(testJobFormat, i))
		if err != nil {
			return err
		}
		files = append(files, fi)
	}
	// Add some files to the archive.
	buf := make([]byte, 1<<32)
	for _, file := range files {
		name, _ := splitNameAndDir(file.Name())
		f, err := w.Create(name)
		if err != nil {
			return err
		}
		eof := false
		for !eof {
			n, err := file.Read(buf)
			if err != nil {
				eof = true
				continue
			}
			_, err = f.Write(buf[:n])
			if err != nil {
				return err
			}
		}
	}
	err = w.Close()
	return err
}

func splitNameAndDir(filename string) (name, dir string) {
	name, dir = filename[strings.LastIndex(filename, fpsep)+1:], filename[:strings.LastIndex(filename, fpsep)-1]
	return
}
