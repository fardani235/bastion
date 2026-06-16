.PHONY: build deb package-windows clean run

build:
	wails build -clean -tags "webkit2_41"

deb: build
	./build/linux/package.sh

package-windows:
	./build/windows/package.sh

clean:
	rm -rf build/bin

run:
	wails dev -tags "webkit2_41"
