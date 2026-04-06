# Contributing to Chassis

Thanks for your interest in contributing to Chassis.

## Getting Started

See the [README](README.md) for build prerequisites and instructions.

### Testing

```sh
task test         # Run tests
task test:race    # Run tests with race detector
task fmt          # Format code
task vet          # Run go vet
task check        # Run all quality checks (fmt, vet, lint, test)
```

## Making Changes

1. Fork the repository
2. Create a branch for your change
3. Make your changes
4. Run `task check` to make sure everything passes
5. Open a pull request against `main`

If you're planning a larger change, open an issue first so we can discuss the approach.

## Code Style

- Run `gofmt` before committing (or `task fmt`)
- Keep comments useful, explain *why*, not *what*
- Follow existing patterns in the codebase
- Stick to the existing architecture: domain types in `core/domain`, interfaces in `core/port`, business logic in `core/service`, and all external access (sysfs, D-Bus, GTK) in `adapter/`

## Reporting Bugs

Open an issue on the repository or post on the [forum](https://forum.defenestra.io/).

## License

By contributing, you agree that your contributions will be licensed under the GPL-3.0-only license.
