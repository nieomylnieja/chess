APP = server
BUILD = .build

.PHONY: all build clean
all: build run

run:
	./$(APP)

build:
	cmake . -B $(BUILD)
	make -C $(BUILD)
	cp $(BUILD)/$(APP) $(APP)

clean:
	rm -rf $(BUILD) $(APP)
