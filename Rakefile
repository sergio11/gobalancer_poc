# Rakefile for GoBalancer POC
# Automation using Podman containerized Go environment

GOLANG_IMAGE = "docker.io/library/golang:1.24-alpine"
PROJECT_DIR = File.expand_path(__dir__)

def podman_run(cmd, env: {})
  env_flags = env.map { |k, v| "-e #{k}=#{v}" }.join(" ")
  system("podman run --rm #{env_flags} -v \"#{PROJECT_DIR}:/app\" -w /app #{GOLANG_IMAGE} #{cmd}")
end

desc "Run all unit and integration tests"
task :test do
  puts "==> Running unit tests in Podman..."
  success = podman_run("go test -v ./internal/...")
  raise "Tests failed!" unless success
end

namespace :test do
  desc "Run tests with coverage and generate HTML report"
  task :coverage do
    puts "==> Running tests with coverage in Podman..."
    cmd = "sh -c 'go test -coverprofile=coverage.out ./internal/... && go tool cover -html=coverage.out -o coverage.html && go tool cover -func=coverage.out'"
    success = podman_run(cmd)
    raise "Coverage test failed!" unless success
    puts "==> Coverage report generated at coverage.html"
  end

  desc "Run End-to-End (E2E) integration tests using Testcontainers and Podman"
  task :e2e do
    puts "==> Running E2E integration tests..."
    sock_path = "/run/user/1000/podman/podman.sock"
    cmd = "go test -v -timeout 10m ./test/e2e/..."
    env = {
      "DOCKER_HOST" => "unix:///var/run/docker.sock",
      "TESTCONTAINERS_RYUK_DISABLED" => "true"
    }
    success = system("podman run --rm #{env.map { |k, v| "-e #{k}=#{v}" }.join(" ")} -v \"#{PROJECT_DIR}:/app\" -v \"#{sock_path}:/var/run/docker.sock\" -w /app #{GOLANG_IMAGE} #{cmd}")
    raise "E2E tests failed!" unless success
  end
end

desc "Build GoBalancer binary"
task :build do
  puts "==> Building GoBalancer binary in Podman..."
  success = podman_run("go build -o bin/gobalancer ./cmd/gobalancer")
  raise "Build failed!" unless success
  puts "==> Binary created at bin/gobalancer"
end

desc "Clean build artifacts"
task :clean do
  puts "==> Cleaning up build artifacts..."
  File.delete("bin/gobalancer") if File.exist?("bin/gobalancer")
  File.delete("coverage.out") if File.exist?("coverage.out")
  File.delete("coverage.html") if File.exist?("coverage.html")
end

task default: [:test]
