package lrutree_test

import (
	"fmt"
	"strings"

	"github.com/vasayxtx/go-lrutree"
)

func Example() {
	// Create a new geographic service with cache size of 100
	geoService := NewGeoService(100)

	// Build geographic hierarchy
	_ = geoService.AddLocation("usa", GeoData{Name: "United States", Type: "country"}, "")
	_ = geoService.AddLocation("ca", GeoData{Name: "California", Type: "state", TaxRate: 7.25, EmergencyStatus: true}, "usa")
	_ = geoService.AddLocation("sf_county", GeoData{Name: "San Francisco County", Type: "county", TaxRate: 0.25}, "ca")
	_ = geoService.AddLocation("sf_city", GeoData{Name: "San Francisco", Type: "city", TaxRate: 0.5}, "sf_county")
	_ = geoService.AddLocation("mission", GeoData{Name: "Mission District", Type: "district"}, "sf_city")
	_ = geoService.AddLocation("tx", GeoData{Name: "Texas", Type: "state", TaxRate: 6.25}, "usa")
	_ = geoService.AddLocation("austin", GeoData{Name: "Austin", Type: "city", TaxRate: 2.0}, "tx")

	printLocationInfo := func(locationID string) {
		path, _ := geoService.GetLocationPath(locationID)
		fmt.Printf("Location: %s\n", path)

		taxRate, _ := geoService.GetEffectiveTaxRate(locationID)
		fmt.Printf("Effective tax rate: %.2f%%\n", taxRate)

		emergency, source, _ := geoService.GetEmergencyStatus(locationID)
		if emergency {
			fmt.Printf("Emergency status: ACTIVE (declared in %s)\n", source)
		} else {
			fmt.Printf("Emergency status: NORMAL\n")
		}
	}

	printLocationInfo("mission") // Should have CA emergency and SF+County+State tax
	fmt.Println("-------------")
	printLocationInfo("austin") // Should have no emergency and TX+City tax

	// Output:
	// Location: United States > California > San Francisco County > San Francisco > Mission District
	// Effective tax rate: 8.00%
	// Emergency status: ACTIVE (declared in California)
	// -------------
	// Location: United States > Texas > Austin
	// Effective tax rate: 8.25%
	// Emergency status: NORMAL
}

// GeoData represents information about a geographic location
type GeoData struct {
	Name            string
	Type            string  // country, state, county, city, district
	TaxRate         float64 // local tax rate (percentage)
	EmergencyStatus bool    // whether area is under emergency declaration
}

// GeoService wraps the LRU tree cache to provide specialized geographic operations
type GeoService struct {
	cache *lrutree.Cache[string, GeoData]
}

const rootID = "earth"

func NewGeoService(cacheSize int) *GeoService {
	cache := lrutree.NewCache[string, GeoData](cacheSize)
	_ = cache.AddRoot(rootID, GeoData{Name: "Earth"})
	return &GeoService{cache: cache}
}

func (g *GeoService) AddLocation(id string, data GeoData, parentID string) error {
	// New location may be added to the database or another source and then added to the cache.
	if parentID == "" {
		parentID = rootID
	}
	return g.cache.Add(id, data, parentID)
}

func (g *GeoService) GetLocationPath(id string) (string, error) {
	branch := g.cache.GetBranch(id)
	if len(branch) == 0 {
		// Location is not cached, may be loaded from the database or another source and added to the cache.
		return "", fmt.Errorf("not found")
	}
	var path []string
	for _, node := range branch[1:] { // Skip the root node
		path = append(path, node.Value.Name)
	}
	return strings.Join(path, " > "), nil
}

// GetEmergencyStatus checks if a location or any of its parent jurisdictions
// has declared an emergency
func (g *GeoService) GetEmergencyStatus(id string) (emergency bool, source string, err error) {
	branch := g.cache.GetBranch(id)
	if len(branch) == 0 {
		// Location is not cached, may be loaded from the database or another source and added to the cache.
		return false, "", fmt.Errorf("not found")
	}

	// We sure that all ancestors are presented in the cache too, so we can just calculate the emergency status
	for _, node := range branch {
		if node.Value.EmergencyStatus {
			return true, node.Value.Name, nil
		}
	}
	return false, "", nil
}

// GetEffectiveTaxRate calculates the total tax rate for a location
// by summing the tax rates from all its parent jurisdictions
func (g *GeoService) GetEffectiveTaxRate(key string) (float64, error) {
	branch := g.cache.GetBranch(key)
	if len(branch) == 0 {
		// Location is not cached, may be loaded from the database or another source and added to the cache
		return 0, fmt.Errorf("not found")
	}

	// We sure that all ancestors are presented in the cache too, so we can just sum their tax rates
	totalRate := 0.0
	for _, node := range branch {
		totalRate += node.Value.TaxRate
	}
	return totalRate, nil
}
