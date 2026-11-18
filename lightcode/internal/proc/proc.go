// Package proc starts the app's console helpers — git, ripgrep — without a
// console window. 轻码 is built as a GUI program (-H windowsgui on
// Windows), so it has no console of its own, and Windows gives every
// console program it starts one: a black window that flashes as the
// background Git refresh or a search runs.
package proc
