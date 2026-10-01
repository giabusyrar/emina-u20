package domain

import (
	"encoding/json"
	"fmt"
	"os"
)

// Status values carried by each config block. Anything other than the two
// "measured" statuses makes the API mark its output provisional.
const (
	StatusWCPA           = "wcpa"           // taken from the Cocory / WCPA material
	StatusBrief          = "brief"          // taken from the brand brief
	StatusPlaceholder    = "placeholder"    // [KARANGAN] in the notebook
	StatusInterpretation = "interpretation" // [INTERPRETASI] in the notebook
	StatusPendingBrand   = "pending_brand"
	StatusDerived        = "derived"
)

// Config is apps/core-engine/config/wcpa.json. Every number the WCPA rules use
// lives there, with the provenance tag it had in the notebook.
type Config struct {
	Version   string `json:"version"`
	Source    string `json:"source"`
	Quadrants []struct {
		Code          string `json:"code"`
		TechnicalName string `json:"technicalName"`
	} `json:"quadrants"`

	Person struct {
		Status     string                `json:"status"`
		ValueHigh  float64               `json:"valueHigh"`
		ValueLow   float64               `json:"valueLow"`
		ChromaHigh float64               `json:"chromaHigh"`
		ChromaLow  float64               `json:"chromaLow"`
		Centres    map[string][2]float64 `json:"centres"`
		Scale      [2]float64            `json:"scale"`
	} `json:"person"`

	Shade struct {
		Status        string                `json:"status"`
		Value         map[string][2]float64 `json:"value"`
		Chroma        map[string][2]float64 `json:"chroma"`
		Centres       map[string][2]float64 `json:"centres"`
		CentresStatus string                `json:"centresStatus"`
		Scale         [2]float64            `json:"scale"`
	} `json:"shade"`

	Temperature struct {
		Status      string  `json:"status"`
		WarmAtLeast float64 `json:"warmAtLeast"`
		CoolAtMost  float64 `json:"coolAtMost"`
	} `json:"temperature"`

	FoundationBands struct {
		Status string `json:"status"`
		Bands  []struct {
			Above float64 `json:"above"`
			Name  string  `json:"name"`
		} `json:"bands"`
	} `json:"foundationBands"`

	Complementary struct {
		Status string `json:"status"`
		Map    map[string]struct {
			Codes       []string `json:"codes"`
			Temperature string   `json:"temperature"`
		} `json:"map"`
	} `json:"complementary"`

	Recommendation struct {
		PerCategory int `json:"perCategory"`
		// Categories the quadrant rules rate. Empty rates every category, as
		// the notebook did.
		Categories []string `json:"categories"`
	} `json:"recommendation"`

	Flags struct {
		UnevenSkinToneLRange float64 `json:"unevenSkinToneLRange"`
	} `json:"flags"`

	HueNames struct {
		// Categories whose shades get a hue name. Empty names every category.
		// The names themselves are the ISCC-NBS standard's (isccnbs.go).
		Categories []string `json:"categories"`
	} `json:"hueNames"`

	Presentation struct {
		Status            string                         `json:"status"`
		DisplayNames      map[string]string              `json:"displayNames"`
		Undertone         map[string]string              `json:"undertone"`
		ValueLabel        map[string]string              `json:"valueLabel"`
		ChromaLabel       map[string]string              `json:"chromaLabel"`
		SeasonEquivalents map[string]map[string][]string `json:"seasonEquivalents"`
	} `json:"presentation"`
}

// LoadConfig reads and validates the WCPA config. A config that cannot drive
// every rule is refused at startup rather than producing half-classified output.
func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading WCPA config %s: %w", path, err)
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parsing WCPA config %s: %w", path, err)
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("WCPA config %s: %w", path, err)
	}
	return &c, nil
}

func (c *Config) validate() error {
	if len(c.Quadrants) == 0 {
		return fmt.Errorf("no quadrants")
	}
	for _, q := range c.Quadrants {
		if _, ok := c.Person.Centres[q.Code]; !ok {
			return fmt.Errorf("person.centres missing %s", q.Code)
		}
		if _, ok := c.Shade.Centres[q.Code]; !ok {
			return fmt.Errorf("shade.centres missing %s", q.Code)
		}
	}
	if c.Person.Scale[0] <= 0 || c.Person.Scale[1] <= 0 || c.Shade.Scale[0] <= 0 || c.Shade.Scale[1] <= 0 {
		return fmt.Errorf("scales must be positive")
	}
	for _, k := range []string{"high", "low"} {
		if _, ok := c.Shade.Value[k]; !ok {
			return fmt.Errorf("shade.value.%s missing", k)
		}
	}
	if _, ok := c.Shade.Chroma["high"]; !ok {
		return fmt.Errorf("shade.chroma.high missing")
	}
	if len(c.FoundationBands.Bands) == 0 {
		return fmt.Errorf("no foundation bands")
	}
	if c.Recommendation.PerCategory <= 0 {
		return fmt.Errorf("recommendation.perCategory must be positive")
	}
	if _, err := ISCCNBSCentroids(); err != nil {
		return err
	}
	return nil
}

// Rates reports whether the quadrant rules rate shades of this category.
func (c *Config) Rates(category string) bool {
	return listed(c.Recommendation.Categories, category)
}

// NamesHue reports whether shades of this category get a hue name.
func (c *Config) NamesHue(category string) bool {
	return listed(c.HueNames.Categories, category)
}

// listed: an empty list means every category.
func listed(list []string, category string) bool {
	if len(list) == 0 {
		return true
	}
	for _, k := range list {
		if k == category {
			return true
		}
	}
	return false
}

// TechnicalName returns the quadrant's technical name for a code.
func (c *Config) TechnicalName(code string) string {
	for _, q := range c.Quadrants {
		if q.Code == code {
			return q.TechnicalName
		}
	}
	return code
}

// PersonProvisional: the person thresholds are still placeholders.
func (c *Config) PersonProvisional() bool {
	return c.Person.Status != StatusWCPA
}

// ComplementaryProvisional: the complementary map is an interpretation.
func (c *Config) ComplementaryProvisional() bool {
	return c.Complementary.Status != StatusWCPA
}
