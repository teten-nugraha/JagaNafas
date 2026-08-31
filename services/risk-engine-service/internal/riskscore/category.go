package riskscore

// Category is the 4-band classification from PRD section 10 step 5.
type Category string

const (
	CategoryAman     Category = "Aman"
	CategoryWaspada  Category = "Waspada"
	CategoryBerisiko Category = "Berisiko"
	CategoryBahaya   Category = "Bahaya"
)

// Level gives categories an ordinal so "naik >=2 kategori" (PRD section 10
// alert rule override) can be checked with simple integer subtraction.
func (c Category) Level() int {
	switch c {
	case CategoryAman:
		return 0
	case CategoryWaspada:
		return 1
	case CategoryBerisiko:
		return 2
	case CategoryBahaya:
		return 3
	default:
		return 0
	}
}

// IsAlertWorthy reports whether this category alone justifies sending an
// alert (Berisiko or Bahaya — PRD section 10 alert rule).
func (c Category) IsAlertWorthy() bool {
	return c == CategoryBerisiko || c == CategoryBahaya
}

// CategoryFromLevel is the inverse of Level, used to decode the category
// stored in the Redis debounce key.
func CategoryFromLevel(level int) (Category, bool) {
	switch level {
	case 0:
		return CategoryAman, true
	case 1:
		return CategoryWaspada, true
	case 2:
		return CategoryBerisiko, true
	case 3:
		return CategoryBahaya, true
	default:
		return CategoryAman, false
	}
}

// Categorize maps a 0-100 score to its band (PRD section 10 step 5).
func Categorize(score float64) Category {
	switch {
	case score >= 81:
		return CategoryBahaya
	case score >= 61:
		return CategoryBerisiko
	case score >= 31:
		return CategoryWaspada
	default:
		return CategoryAman
	}
}
