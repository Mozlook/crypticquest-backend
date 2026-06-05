package handlers

// Field length caps for admin write endpoints. The 1 MiB body cap
// (MaxBytesReader) is the transport backstop; these bound individual fields so a
// single value can't be absurdly large. Generous on purpose — they guard against
// abuse, not legitimate content.
const (
	maxTitleLen       = 200
	maxFlagLen        = 200
	maxLevelDescLen   = 20000 // puzzle narratives can be long
	maxToolDescLen    = 2000
	maxToolContentLen = 2000 // a URL or a file path
	maxHintLen        = 1000
)
