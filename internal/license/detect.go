// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package license

import (
	"archive/zip"
	"fmt"
	"io/ioutil"
	"log"
	"path"
	"sort"
	"strings"

	"github.com/google/licensecheck"
	"golang.org/x/discovery/internal/derrors"
	"golang.org/x/discovery/internal/thirdparty/module"
)

const (
	// classifyThreshold is the minimum confidence percentage/threshold
	// to classify a license
	classifyThreshold = 90

	// coverageThreshold is the minimum percentage of the file that must contain license text.
	coverageThreshold = 90

	// maxLicenseSize is the maximum allowable size (in bytes) for a license
	// file.
	maxLicenseSize = 1e7
)

// licenseFileNames defines the set of filenames to be considered for license
// extraction.
var licenseFileNames = map[string]bool{
	"LICENSE":     true,
	"LICENSE.md":  true,
	"LICENSE.txt": true,
	"COPYING":     true,
	"COPYING.md":  true,
	"COPYING.txt": true,
}

// isVendoredFile reports if the given file is in a proper subdirectory nested
// under a 'vendor' directory, to allow for Go packages named 'vendor'.
//
// e.g. isVendoredFile("vendor/LICENSE") == false, and
//      isVendoredFile("vendor/foo/LICENSE") == true
func isVendoredFile(name string) bool {
	var vendorOffset int
	if strings.HasPrefix(name, "vendor/") {
		vendorOffset = len("vendor/")
	} else if i := strings.Index(name, "/vendor/"); i >= 0 {
		vendorOffset = i + len("/vendor/")
	} else {
		// no vendor directory
		return false
	}
	// check if the file is in a proper subdirectory of vendor
	return strings.Contains(name[vendorOffset:], "/")
}

// Detect searches for possible license files in a subdirectory within the
// provided zip path, runs them against a license classifier, and provides all
// licenses with a confidence score that meets a confidence threshold.
//
// It returns an error if the given file path is invalid, if the uncompressed
// size of the license file is too large, if a license is discovered outside of
// the expected path, or if an error occurs during extraction.
func Detect(contentsDir string, r *zip.Reader) (_ []*License, err error) {
	defer derrors.Add(&err, "Detect(%q)", contentsDir)
	var licenses []*License
	for _, f := range r.File {
		if !licenseFileNames[path.Base(f.Name)] || isVendoredFile(f.Name) {
			// Only consider licenses with an acceptable file name, and not in the
			// vendor directory.
			continue
		}
		if err := module.CheckFilePath(f.Name); err != nil {
			return nil, fmt.Errorf("module.CheckFilePath(%q): %v", f.Name, err)
		}
		prefix := ""
		if contentsDir != "" {
			prefix = contentsDir + "/"
		}
		if !strings.HasPrefix(f.Name, prefix) {
			return nil, fmt.Errorf("potential license file %q found outside of the expected path %s", f.Name, contentsDir)
		}
		if f.UncompressedSize64 > maxLicenseSize {
			return nil, fmt.Errorf("potential license file %q exceeds maximum uncompressed size %d", f.Name, int(1e7))
		}

		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("f.Open() for %q: %v", f.Name, err)
		}
		defer rc.Close()

		contents, err := ioutil.ReadAll(rc)
		if err != nil {
			return nil, fmt.Errorf("ioutil.ReadAll(rc) for %q: %v", f.Name, err)
		}

		// At this point we have a valid license candidate, and so expect a match.
		// If we don't find one, we must return an unknown license.
		matched := false
		filePath := strings.TrimPrefix(f.Name, prefix)
		cov, ok := licensecheck.Cover(contents, licensecheck.Options{})
		if ok && cov.Percent >= coverageThreshold {
			matchedTypes := make(map[string]bool)

			for _, m := range cov.Match {
				if m.Percent >= classifyThreshold {
					if matchedTypes[m.Name] {
						log.Printf("WARNING: found license type %s more than once in %s", m.Name, filePath)
					}
					matchedTypes[m.Name] = true
				}
			}
			if len(matchedTypes) > 0 {
				matched = true
				var typs []string
				for t := range matchedTypes {
					typs = append(typs, t)
				}
				sort.Strings(typs)
				licenses = append(licenses, &License{
					Metadata: &Metadata{
						Types:    typs,
						FilePath: filePath,
					},
					Contents: contents,
				})
			}
		}
		if !matched {
			licenses = append(licenses, &License{
				Metadata: &Metadata{FilePath: filePath},
			})

		}
	}
	return licenses, nil
}
