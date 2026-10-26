package food

import "strings"

// diacriticFolds maps lowercase Latin letters with diacritics to their plain
// ASCII form. The standard library has no Unicode decomposition, so the
// common Latin-1 and Latin Extended-A letters are listed explicitly.
var diacriticFolds = strings.NewReplacer(
	"ß", "ss", "æ", "ae", "œ", "oe", "ø", "o", "ł", "l", "đ", "d", "ı", "i",
	"à", "a", "á", "a", "â", "a", "ã", "a", "ä", "a", "å", "a", "ā", "a", "ă", "a", "ą", "a",
	"ç", "c", "ć", "c", "ĉ", "c", "č", "c",
	"ď", "d",
	"è", "e", "é", "e", "ê", "e", "ë", "e", "ē", "e", "ė", "e", "ę", "e", "ě", "e",
	"ğ", "g", "ģ", "g",
	"ì", "i", "í", "i", "î", "i", "ï", "i", "ī", "i", "į", "i",
	"ķ", "k",
	"ĺ", "l", "ļ", "l", "ľ", "l",
	"ñ", "n", "ń", "n", "ņ", "n", "ň", "n",
	"ò", "o", "ó", "o", "ô", "o", "õ", "o", "ö", "o", "ō", "o", "ő", "o",
	"ŕ", "r", "ř", "r",
	"ś", "s", "ş", "s", "š", "s", "ș", "s",
	"ţ", "t", "ť", "t", "ț", "t",
	"ù", "u", "ú", "u", "û", "u", "ü", "u", "ū", "u", "ů", "u", "ű", "u", "ų", "u",
	"ý", "y", "ÿ", "y",
	"ź", "z", "ż", "z", "ž", "z",
)

// normalizeName lowercases, trims, collapses whitespace and folds Latin
// diacritics so that "Möhre", "mohre" and "MÖHRE" compare equal.
func normalizeName(s string) string {
	s = diacriticFolds.Replace(strings.ToLower(s))
	return strings.Join(strings.Fields(s), " ")
}
