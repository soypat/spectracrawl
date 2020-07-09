package cmd

import wd "github.com/fedesog/webdriver"

func query(s *wd.Session, querySelector string) (wd.WebElement, error) {
	return s.FindElement("css selector", querySelector)
}

func leftClickSelector(s *wd.Session, querySelector string) error {
	//var m wd.MouseButton = 0 // left click
	elem, err := query(s, querySelector) // button selector
	if err != nil {
		return err
	}
	return elem.Click()
}
