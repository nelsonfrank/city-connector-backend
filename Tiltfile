# Tiltfile for city-connector backend local development

# Load Kubernetes YAMLs
k8s_yaml(listdir('infra/k8s'))

# Build Go Services
docker_build('nelsonfrank/routing:latest', 'services/routing')
docker_build('nelsonfrank/search:latest', 'services/search')
docker_build('nelsonfrank/realtime:latest', 'services/realtime')

# Build Python Ingestion Job
docker_build('nelsonfrank/ingestion:latest', 'services/ingestion')

# Port forward Kong proxy and admin interfaces to localhost
k8s_resource('kong', port_forwards=['8000:8000', '8001:8001'])
