package claims_test

import (
	"fmt"

	"github.com/sebastienrousseau/draft/internal/claims"
)

func ExampleParse() {
	source := "The method used 5x fewer tokens than the baseline."
	raw := "CLAIM: used 5x fewer tokens\nSOURCE_QUOTE: \"used 5x fewer tokens\"\nTYPE: result\nSTRENGTH: demonstrated\n---"
	records, dropped := claims.Parse(raw, source)
	fmt.Printf("%d verified, %d dropped\n", len(records), dropped)
	// Output: 1 verified, 0 dropped
}
