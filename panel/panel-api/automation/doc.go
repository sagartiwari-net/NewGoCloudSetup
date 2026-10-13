// Package automation is the shared cookie shape and the three steps every provider runs:
// check the saved session, log in when it is dead, then capture the tool cookie.
//
// Each portal has its own folder. Markhor is automation/markhor. Azad, Seoshope, and
// Toolzbuy will be sibling folders. Semrush is a job that points at one of those
// providers. It does not get a folder of its own, because Azad Semrush and
// Toolzbuy Semrush would otherwise look like the same kind of file.
package automation
