package app

// Context is passed to header/footer/content helpers each frame.
type Context struct {
	// Width and Height are the full terminal dimensions.
	Width  int
	Height int
	// MainHeight is the rows available to Content after header/footer chrome.
	MainHeight int
	// OverlayOpen reports whether a modal overlay currently captures input.
	OverlayOpen bool
	// Chrome holds the measured header/main/footer regions.
	Chrome Region
}
