# Rakefile for GoBalancer POC
# Automation using Podman containerized Go environment

require "open3"
require "shellwords"

GOLANG_IMAGE = "docker.io/library/golang:1.24-alpine"
PROJECT_DIR = File.expand_path(__dir__)
MIN_COVERAGE = 98.0

def podman_run(cmd, env: {})
  env_args = env.flat_map { |k, v| ["-e", "#{k}=#{v}"] }
  full_cmd = ["podman", "run", "--rm"] + env_args + ["-v", "#{PROJECT_DIR}:/app", "-w", "/app", GOLANG_IMAGE] + cmd.shellsplit
  stdout, stderr, status = Open3.capture3(*full_cmd)
  puts stdout unless stdout.empty?
  $stderr.puts stderr unless stderr.empty?
  status.success?
end

desc "Run all unit and integration tests"
task :test => ["test:coverage"]

namespace :test do
  desc "Run tests with coverage and enforce minimum 98% coverage threshold"
  task :coverage do
    puts "==> Running tests with coverage in Podman..."

    test_cmd = ["sh", "-c", "go test -coverprofile=coverage.out ./internal/... && go tool cover -func=coverage.out"]
    full_cmd = ["podman", "run", "--rm", "-v", "#{PROJECT_DIR}:/app", "-w", "/app", GOLANG_IMAGE] + test_cmd

    stdout, stderr, status = Open3.capture3(*full_cmd)
    puts stdout
    $stderr.puts stderr unless stderr.empty?

    unless status.success?
      raise "Unit tests failed!"
    end

    match = stdout.match(/total:\s+\(statements\)\s+([\d\.]+)%/)
    if match
      coverage = match[1].to_f
      puts "--------------------------------------------------------"
      puts "==> Total Code Coverage: #{coverage}% (Threshold: #{MIN_COVERAGE}%)"
      puts "--------------------------------------------------------"

      podman_run("go tool cover -html=coverage.out -o coverage.html")

      if coverage < MIN_COVERAGE
        raise "Coverage check FAILED: Total coverage of #{coverage}% is below the required #{MIN_COVERAGE}% threshold!"
      else
        puts "==> Coverage check PASSED successfully!"
      end
    else
      raise "Unable to parse code coverage output."
    end
  end

  desc "Run End-to-End (E2E) integration tests using Testcontainers and Podman"
  task :e2e do
    puts "==> Running E2E integration tests..."
    sock_path = "/run/user/1000/podman/podman.sock"
    env = {
      "DOCKER_HOST" => "unix:///var/run/docker.sock",
      "TESTCONTAINERS_RYUK_DISABLED" => "true"
    }
    test_cmd = ["sh", "-c", "go test -v -timeout 10m ./test/e2e/..."]
    env_args = env.flat_map { |k, v| ["-e", "#{k}=#{v}"] }
    full_cmd = ["podman", "run", "--rm"] + env_args + ["-v", "#{PROJECT_DIR}:/app", "-v", "#{sock_path}:/var/run/docker.sock", "-w", "/app", GOLANG_IMAGE] + test_cmd

    stdout, stderr, status = Open3.capture3(*full_cmd)
    puts stdout
    $stderr.puts stderr unless stderr.empty?
    raise "E2E tests failed!" unless status.success?
  end
end

desc "Build GoBalancer binary"
task :build do
  puts "==> Building GoBalancer binary in Podman..."
  unless podman_run("go build -o bin/gobalancer ./cmd/gobalancer")
    raise "Build failed!"
  end
  puts "==> Binary created at bin/gobalancer"
end

desc "Clean build artifacts"
task :clean do
  puts "==> Cleaning build artifacts..."
  File.delete("bin/gobalancer") if File.exist?("bin/gobalancer")
  File.delete("coverage.out") if File.exist?("coverage.out")
  File.delete("coverage.html") if File.exist?("coverage.html")
end

task default: [:test]
