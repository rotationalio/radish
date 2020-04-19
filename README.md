# Radish

Radish is a stateless task queue and worker protocol that can maximize the resources of a single node by increasing and decreasing the number of worker go routines that can handle tasks. The radish server allows users to scale the number of workers that can handle generic tasks, add tasks to the queue, and reports metrics to prometheus for easy tracking and management.
