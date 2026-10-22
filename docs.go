package actionsqaci

import "embed"

// Pages holds the rule pages and the identifier index, rooted at the repository so a page is
// at docs/rules/<identifier>.md. The program reads this copy, so rule-doc works offline.
//
//go:embed docs/rules/*.md
var Pages embed.FS
