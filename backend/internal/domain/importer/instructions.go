package importer

import "regexp"

// pleasantryRE matches a step that is only a sign-off ("Guten Appetit!", "Ich
// wünsche dir einen guten Appetit!", "Enjoy!"). A step that says more than
// that is a real step and never matches.
var pleasantryRE = regexp.MustCompile(`(?i)^\W*((ich|wir) wünsche[n]? (dir|euch|ihnen) )?(einen )?(guten appetit|bon app[eé]tit|buon appetito|enjoy( your meal)?|mahlzeit|viel spa(ß|ss)( beim essen)?|happy cooking)\W*$`)

// dropClosingPleasantry returns a copy of steps without a trailing sign-off.
// A recipe whose only step is one keeps it: the step list is never emptied.
func dropClosingPleasantry(steps []string) []string {
	out := append([]string{}, steps...)
	if n := len(out); n > 1 && pleasantryRE.MatchString(out[n-1]) {
		out = out[:n-1]
	}
	return out
}
