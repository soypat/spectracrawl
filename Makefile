buildflags = -ldflags="-s -w" -i
binname = spectracrawl
distr:
	go build ${buildflags} -o bin/${binname}.exe
	cp README.md README.txt
	zip ${binname} -j bin/${binname}.exe README.txt .${binname}.yml
	rm README.txt
	zip ${binname} bin/chromedriver.exe
mkbin:
	mkdir bin
