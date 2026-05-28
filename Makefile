.PHONY: proto proto-lint proto-format proto-breaking proto-deps

proto:
	buf generate

proto-lint:
	buf lint

proto-format:
	buf format -w

proto-breaking:
	buf breaking --against '.git#branch=main'

proto-deps:
	buf dep update
