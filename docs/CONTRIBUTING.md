# Contributing to Kestrel (Silver Go IMAP)

Thank you for your interest in contributing to the Kestrel (Silver Go IMAP) project! This document provides guidelines and instructions for contributing.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [How to Contribute](#how-to-contribute)
- [Coding Standards](#coding-standards)
- [Testing](#testing)
- [Submitting Changes](#submitting-changes)

## Code of Conduct

This project adheres to a code of conduct that all contributors are expected to follow. Please be respectful and constructive in all interactions.

## Getting Started

1. **Fork the repository** on GitHub
2. **Clone your fork** locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/kestrel.git
   cd kestrel
   ```
3. **Add the upstream repository**:
   ```bash
   git remote add upstream https://github.com/LSFLK/kestrel.git
   ```

## How to Contribute

### Types of Contributions

We welcome various types of contributions:

- **Bug fixes**: Help us squash bugs in the codebase
- **New features**: Implement new IMAP extensions or functionality
- **Documentation**: Improve README, godoc comments, or examples
- **Tests**: Add test coverage for existing functionality
- **Performance improvements**: Optimize existing code

### Before You Start

1. **Check existing issues** to see if your bug/feature is already being discussed
2. **Open an issue** to discuss substantial changes before starting work
3. **Keep changes focused**: One pull request per feature or bug fix

## Coding Standards

### Go Code Style

- Follow standard Go conventions and idioms
- Use `gofmt` to format your code
- Run `go vet` to catch common mistakes
- Consider using `golangci-lint` for comprehensive linting

### Code Organization

- Keep functions small and focused
- Use meaningful variable and function names
- Add godoc comments for exported types, functions, and packages
- Group related functionality together

### Documentation

- Document all exported functions, types, and constants
- Use complete sentences in comments
- Provide examples for complex functionality
- Keep documentation up to date with code changes

### Example godoc Comment

```go
// ParseMessage parses an IMAP message from the provided reader.
// It returns a Message struct or an error if parsing fails.
//
// Example:
//   msg, err := ParseMessage(reader)
//   if err != nil {
//       return err
//   }
func ParseMessage(r io.Reader) (*Message, error) {
    // implementation
}
```

## Testing

### Writing Tests

- Write tests for all new functionality
- Ensure existing tests pass before submitting
- Use table-driven tests where appropriate
- Test edge cases and error conditions

### Test Coverage

Check test coverage with:

```bash
go test -cover ./...
```

Generate a detailed coverage report:

```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Example Test Structure

```go
func TestParseMessage(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    *Message
        wantErr bool
    }{
        {
            name:  "valid message",
            input: "...",
            want:  &Message{...},
        },
        // more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseMessage(strings.NewReader(tt.input))
            if (err != nil) != tt.wantErr {
                t.Errorf("ParseMessage() error = %v, wantErr %v", err, tt.wantErr)
                return
            }
            if !reflect.DeepEqual(got, tt.want) {
                t.Errorf("ParseMessage() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

## Submitting Changes

### Commit Messages

Write clear, descriptive commit messages:

Use the conventional commit message. Follow this [guide](https://www.conventionalcommits.org/en/v1.0.0/).

### Pull Request Process

1. **Update your fork** with the latest upstream changes:

   ```bash
   git fetch upstream
   git rebase upstream/master
   ```

2. **Create a feature branch**:

   ```bash
   git checkout -b feature/my-new-feature
   ```

3. **Make your changes** and commit them with clear messages

4. **Push to your fork**:

   ```bash
   git push origin feature/my-new-feature
   ```

5. **Open a pull request** on GitHub with:
   - A clear title and description
   - Reference to any related issues
   - Summary of changes made
   - Any breaking changes highlighted

6. **Respond to feedback** and update your PR as needed

### Pull Request Checklist

Before submitting, ensure:

- [ ] Core IMAP commands tested (`LOGIN`, `CAPABILITY`, `LIST`, `SELECT`, `FETCH`, `LOGOUT`).
- [ ] Authentication is tested.
- [ ] Docker build & run validated.
- [ ] Configuration loading verified for default and custom paths.
- [ ] Persistent storage with Docker volume verified.
- [ ] Error handling and logging verifie
- [ ] Documentation updated (README, config samples).

## Questions?

If you have questions about contributing, feel free to:

- Open an issue with the "question" label
- Check existing documentation and issues first
- Be patient and respectful when asking for help

## License

By contributing to this project, you agree that your contributions will be licensed under the Apache License 2.0, and will comply with its terms and conditions.

See the [LICENSE](LICENSE) file for details.

---

Thank you for contributing to Go IMAP! Your efforts help make this project better for everyone.
