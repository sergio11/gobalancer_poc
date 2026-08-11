# Rakefile for GoBalancer POC
# Automation using Podman containerized Go environment

require "open3"
require "shellwords"

GOLANG_IMAGE = "docker.io/library/golang:1.24-alpine"
# Debian-based image includes gcc, required for `go test -race` (cgo).
GOLANG_RACE_IMAGE = "docker.io/library/golang:1.24"
PROJECT_DIR = File.expand_path(__dir__)
MIN_COVERAGE = 98.0
PODMAN_SOCKET = "/run/user/1000/podman/podman.sock"

def podman_run(cmd, env: {}, image: GOLANG_IMAGE, mounts: [])
  env_args = env.flat_map { |k, v| ["-e", "#{k}=#{v}"] }
  mount_args = mounts.flat_map { |m| ["-v", m] }
  full_cmd = ["podman", "run", "--rm"] + env_args + mount_args + ["-v", "#{PROJECT_DIR}:/app", "-w", "/app", image] + cmd.shellsplit
  stdout, stderr, status = Open3.capture3(*full_cmd)
  puts stdout unless stdout.empty?
  $stderr.puts stderr unless stderr.empty?
  status.success?
end

def e2e_env
  {
    "DOCKER_HOST" => "unix:///var/run/docker.sock",
    "TESTCONTAINERS_RYUK_DISABLED" => "true"
  }
end

desc "Run all tests: unit + coverage + race detector + E2E"
task :test do
  puts "==> Running unit tests with coverage..."
  test_cmd = ["sh", "-c", "go test -coverprofile=coverage.out ./internal/... && go tool cover -func=coverage.out"]
  full_cmd = ["podman", "run", "--rm", "-v", "#{PROJECT_DIR}:/app", "-w", "/app", GOLANG_IMAGE] + test_cmd

  stdout, stderr, status = Open3.capture3(*full_cmd)
  puts stdout
  $stderr.puts stderr unless stderr.empty?
  raise "Unit tests failed!" unless status.success?

  match = stdout.match(/total:\s+\(statements\)\s+([\d\.]+)%/)
  raise "Unable to parse code coverage output." unless match
  coverage = match[1].to_f
  puts "--------------------------------------------------------"
  puts "==> Total Code Coverage: #{coverage}% (Threshold: #{MIN_COVERAGE}%)"
  puts "--------------------------------------------------------"

  if coverage < MIN_COVERAGE
    raise "Coverage check FAILED: #{coverage}% is below the required #{MIN_COVERAGE}% threshold!"
  end
  puts "==> Coverage check PASSED!"

  podman_run("go tool cover -html=coverage.out -o coverage.html")

  puts "==> Running unit tests with the race detector..."
  unless podman_run("go test -race -count=1 ./internal/...", image: GOLANG_RACE_IMAGE)
    raise "Race detector unit tests failed!"
  end
  puts "==> Race detector unit tests PASSED!"

  puts "==> Running E2E integration tests..."
  unless podman_run(
    "go test -v -timeout 10m ./test/e2e/...",
    env: e2e_env,
    mounts: ["#{PODMAN_SOCKET}:/var/run/docker.sock"]
  )
    raise "E2E tests failed!"
  end
  puts "==> E2E tests PASSED!"

  puts "==> Running E2E tests with the race detector..."
  unless podman_run(
    "go test -race -count=1 -timeout 15m ./test/e2e/...",
    env: e2e_env,
    image: GOLANG_RACE_IMAGE,
    mounts: ["#{PODMAN_SOCKET}:/var/run/docker.sock"]
  )
    raise "Race detector E2E tests failed!"
  end
  puts "==> Race detector E2E tests PASSED!"

  puts "==> All tests passed!"
end

desc "Run all tests with the race detector enabled"
task "test:race" do
  puts "==> Running unit tests with the race detector..."
  unless podman_run("go test -race -count=1 ./internal/...", image: GOLANG_RACE_IMAGE)
    raise "Race detector unit tests failed!"
  end
  puts "==> Race detector unit tests PASSED!"

  puts "==> Running E2E tests with the race detector..."
  unless podman_run(
    "go test -race -count=1 -timeout 15m ./test/e2e/...",
    env: e2e_env,
    image: GOLANG_RACE_IMAGE,
    mounts: ["#{PODMAN_SOCKET}:/var/run/docker.sock"]
  )
    raise "Race detector E2E tests failed!"
  end
  puts "==> Race detector E2E tests PASSED!"
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

# =============================================================================
# Demo Tasks - Podman-Compose based L7 Load Balancer demonstration
# =============================================================================

DEMO_DIR = File.join(PROJECT_DIR, "demo")
LB_URL = "http://localhost:8080"

def demo_compose(cmd)
  full_cmd = ["podman-compose", "-f", File.join(DEMO_DIR, "podman-compose.yml")] + cmd.shellsplit
  stdout, stderr, status = Open3.capture3(*full_cmd)
  puts stdout unless stdout.empty?
  $stderr.puts stderr unless stderr.empty?
  status.success?
end

def curl_get(url)
  stdout, stderr, status = Open3.capture3("curl.exe", "-s", url)
  status.success? ? stdout.strip : nil
end

def curl_post(url, body)
  stdout, stderr, status = Open3.capture3("curl.exe", "-s", "-X", "POST", url, "-H", "Content-Type: application/json", "-d", body)
  status.success? ? stdout.strip : nil
end

def extract_backend(html)
  match = html.match(/Backend #(\d)/)
  match ? match[1].to_i : nil
end

def wait_for_lb(seconds = 15)
  seconds.times do |i|
    resp = curl_get("#{LB_URL}/health")
    return true if resp&.include?("UP")
    sleep 1
  end
  false
end

def print_header(msg)
  puts ""
  puts "=" * 76
  puts "  #{msg}"
  puts "=" * 76
  puts ""
end

def print_step(msg)
  puts "  ▸ #{msg}"
end

def print_info(msg)
  puts "    → #{msg}"
end

def print_success(msg)
  puts "    ✓ #{msg}"
end

def print_error(msg)
  puts "    ✗ #{msg}"
end

def print_distribution(counts, total)
  puts ""
  puts "  ┌────────────┬────────────┬──────────────────────┐"
  puts "  │ Backend    │ Peticiones │ Proporcion           │"
  puts "  ├────────────┼────────────┼──────────────────────┤"
  (1..3).each do |i|
    key = "backend-#{i}"
    c = counts[key] || 0
    bar = "█" * c
    puts "  │ #{key}  │ #{c.to_s.rjust(10)} │ #{bar.ljust(20)} │"
  end
  puts "  └────────────┴────────────┴──────────────────────┘"
end

namespace :demo do
  desc "Stop and remove demo containers"
  task :stop do
    print_header "STOP - Deteniendo contenedores"
    demo_compose("down")
    print_success "Contenedores detenidos"
  end

  desc "Run full interactive demo (build, up, round-robin, weighted, failover, metrics)"
  task :run do
    print_header "BUILD - Construyendo imagenes"
    unless demo_compose("build --pull")
      raise "Demo build failed!"
    end
    print_success "Imagenes construidas"

    print_header "UP - Iniciando contenedores"
    demo_compose("up -d")
    sleep 3
    unless wait_for_lb
      puts `podman-compose -f #{File.join(DEMO_DIR, "podman-compose.yml")} logs gobalancer 2>&1`
      raise "GoBalancer no responde"
    end
    print_success "GoBalancer escuchando en #{LB_URL}"

    print_header "FASE 1: ROUND ROBIN"
    print_step "Algoritmo: Round Robin (ciclico 1→2→3→1→2→3...)"
    print_step "Enviando 12 peticiones..."
    puts ""

    counts = { "backend-1" => 0, "backend-2" => 0, "backend-3" => 0 }
    12.times do |i|
      resp = curl_get("#{LB_URL}/")
      b = extract_backend(resp)
      counts["backend-#{b}"] += 1 if b
      puts "  Peticion #{(i + 1).to_s.rjust(2)}: backend-#{b}"
    end
    print_distribution(counts, 12)

    print_header "FASE 2: WEIGHTED ROUND ROBIN"
    print_step "Cambiando configuracion a Weighted Round Robin..."
    curl_post("#{LB_URL}/api/reload", '{"config_path": "/app/configs/config-weighted.yaml"}')
    sleep 2
    print_step "Pesos: backend-1=5, backend-2=3, backend-3=1"
    print_step "Enviando 12 peticiones..."
    puts ""

    counts = { "backend-1" => 0, "backend-2" => 0, "backend-3" => 0 }
    12.times do |i|
      resp = curl_get("#{LB_URL}/")
      b = extract_backend(resp)
      counts["backend-#{b}"] += 1 if b
      puts "  Peticion #{(i + 1).to_s.rjust(2)}: backend-#{b}"
    end
    print_distribution(counts, 12)

    print_header "FASE 3: HEALTH CHECK + FAILOVER"
    print_step "Deteniendo backend-2..."
    `podman stop backend-2`
    print_step "Esperando deteccion del fallo (~15s)..."
    15.times do |i|
      print "\r    ⏳ Esperando... #{(i + 1).to_s.rjust(2)}s/15s"
      sleep 1
    end
    puts ""

    print_step "Enviando 6 peticiones despues del failover..."
    puts ""
    counts = { "backend-1" => 0, "backend-2" => 0, "backend-3" => 0 }
    6.times do |i|
      resp = curl_get("#{LB_URL}/")
      b = extract_backend(resp)
      counts["backend-#{b}"] += 1 if b
      puts "  Peticion #{(i + 1).to_s.rjust(2)}: backend-#{b}"
    end

    if counts["backend-2"] == 0
      print_success "FAILOVER EXITOSO: backend-2 no recibe trafico"
    else
      print_error "FAILOVER FALLO: backend-2 aun recibe trafico"
    end

    print_step "Recuperando backend-2..."
    `podman start backend-2`
    sleep 5
    resp = curl_get("#{LB_URL}/")
    b = extract_backend(resp)
    print_info "Backend recuperado, ultima peticion: backend-#{b}"

    print_header "FASE 4: METRICAS"
    print_step "Admin API:"
    puts ""
    stats = curl_get("#{LB_URL}/api/stats")
    puts stats
    puts ""
    print_step "Prometheus Metrics:"
    puts ""
    metrics = curl_get("#{LB_URL}/metrics")
    puts metrics

    print_header "FASE 5: POR QUE GO?"
    puts <<~INFO
      ┌─────────────────────────────────────────────────────────────────────┐
      │                    GOROUTINES VS THREADS                           │
      ├─────────────────────────────────────────────────────────────────────┤
      │  Recurso           │ Goroutines (Go)     │ Threads (Java/C++)      │
      │  ─────────────────┼────────────────────┼───────────────────────    │
      │  Stack inicial     │ ~2.5 KB            │ ~1 MB                    │
      │  10,000 instancias │ ~25 MB             │ ~10 GB                   │
      │  Creacion         │ < 1 μs              │ ~100 μs                  │
      │  Cambio contexto   │ ~200 ns            │ ~10 μs                   │
      │  Scheduler         │ Go runtime (M:N)   │ OS kernel (1:1)          │
      └─────────────────────────────────────────────────────────────────────┘

      ┌─────────────────────────────────────────────────────────────────────┐
      │                    COMPARATIVA DE CONTENEDORES                      │
      ├─────────────────────────────────────────────────────────────────────┤
      │  Contenedor          │ Tamano  │ Startup  │ Memoria Idle            │
      │  ──────────────────┼────────┼─────────┼──────────────────         │
      │  GoBalancer (Go)    │ ~15 MB  │ <100 ms  │ ~8 MB                   │
      │  nginx:alpine       │ ~25 MB  │ ~2 s     │ ~5 MB                   │
      │  Python + Flask     │ ~100 MB │ ~3 s     │ ~30 MB                  │
      │  Java + Spring      │ ~300 MB │ ~5 s     │ ~100 MB                 │
      └─────────────────────────────────────────────────────────────────────┘
    INFO

    print_header "DEMO COMPLETADA"
    print_info "Para detener: rake demo:stop"
  end
end

desc "Run interactive demo (alias for demo:run)"
task :demo => ["demo:run"]

task default: [:test]
