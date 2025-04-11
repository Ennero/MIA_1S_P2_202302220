<template>
    <div class="container py-5">
        <div class="row justify-content-center">
            <div class="col-md-10 col-lg-8">

                <div class="text-center mb-4">
                    <h1 class="display-5 fw-bold text-primary">Visualizador del Sistema</h1>
                    <p class="lead text-muted">Seleccione el disco que desea visualizar:</p>
                    <hr class="my-4 text-primary opacity-75">
                </div>

                <div v-if="isLoading" class="text-center my-5">
                    <div class="spinner-border text-primary" role="status">
                        <span class="visually-hidden">Cargando discos...</span>
                    </div>
                    <p class="mt-2">Cargando discos...</p>
                </div>

                <div v-else-if="errorMessage" class="alert alert-danger text-center">
                    <i class="bi bi-exclamation-triangle-fill me-2"></i> {{ errorMessage }}
                </div>

                <div v-else-if="disks.length > 0" class="row g-4 justify-content-center">
                    <div v-for="disk in disks" :key="disk.path" class="col-6 col-sm-4 col-md-3 text-center">
                        <div class="disk-icon-wrapper p-3 border rounded bg-light shadow-sm" @click="selectDisk(disk)"
                            style="cursor: pointer;">
                            <i class="bi bi-hdd-fill text-secondary display-4"></i>
                            <p class="mt-2 mb-0 fw-bold small text-truncate" :title="disk.name">{{ disk.name }}</p>
                        </div>
                    </div>
                </div>

                <div v-else class="alert alert-warning text-center">
                    <i class="bi bi-info-circle-fill me-2"></i> No hay discos registrados o no se pudo obtener la
                    información.
                </div>

                <div class="mt-4 text-center">
                    <button @click="volverAConsola" class="btn btn-secondary">
                        <i class="bi bi-arrow-left me-2"></i>Volver a Consola Principal
                    </button>
                </div>

            </div>
        </div>
    </div>
</template>

<script>
export default {
    name: 'DiskPage',
    data() {
        return {
            disks: [],      
            isLoading: false,  
            errorMessage: ''   
        };
    },
    methods: {
        // Método para obtener la info de discos del backend
        async fetchDisks() {
            this.isLoading = true;
            this.errorMessage = '';
            this.disks = [];
            console.log("Enviando comando 'disks' al backend...");

            try {
                const response = await fetch('http://localhost:3001/', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ command: "disks" }),
                });

                const data = await response.json();

                if (!response.ok || data.error) {
                    const errorMsg = data.error || data.output || `Error HTTP ${response.status}`;
                    throw new Error(`Error obteniendo lista de discos: ${errorMsg}`);
                }

                console.log("Respuesta recibida:", data.output);
                this.parseAndSetDisks(data.output); // Llamar al parser

            } catch (error) {
                console.error("Error en fetchDisks:", error);
                this.errorMessage = error.message || "Error de conexión o respuesta inválida.";
            } finally {
                this.isLoading = false;
            }
        },

        // Método para parsear el string devuelto por el backend (CORREGIDO)
        parseAndSetDisks(outputString) {
            // Validaciones iniciales
            if (!outputString || typeof outputString !== 'string') {
                console.warn("String de salida inválido:", outputString);
                this.errorMessage = "Respuesta inválida del servidor (no es string).";
                this.disks = [];
                return;
            }
            const prefix = "DISKS:\n"; // Prefijo esperado
            if (!outputString.startsWith(prefix)) {
                console.warn("String de salida sin prefijo esperado:", outputString);
                this.errorMessage = "Respuesta inválida del servidor (formato inesperado).";
                this.disks = [];
                return;
            }

            // Usar slice para quitar prefijo 
            let dataString = outputString.slice(prefix.length);

            if (dataString.trim() === "") {
                console.log("No hay discos registrados según el backend.");
                this.disks = [];
                return;
            }

            const diskEntries = dataString.split(';');
            const parsedDisks = [];

            for (const entry of diskEntries) {
                const trimmedEntry = entry.trim();
                if (trimmedEntry === "") continue;

                const fields = trimmedEntry.split(',');
                if (fields.length !== 5) {
                    console.warn("Entrada de disco con formato incorrecto (campos != 5), saltando:", entry);
                    continue;
                }

                // Usar trim() en cada campo
                const diskName = fields[0].trim();
                const diskPath = fields[1].trim();
                const diskSizeStr = fields[2].trim();
                const diskFit = fields[3].trim();
                const mountedStr = fields[4].trim();

                // --- CORRECCIÓN 2: Usar parseInt y isNaN ---
                const diskSize = parseInt(diskSizeStr, 10); // Base 10
                if (isNaN(diskSize)) {
                    console.warn(`Tamaño inválido (no es número) para disco '${diskName}', saltando:`, diskSizeStr);
                    continue;
                }

                // Parsear particiones montadas
                let mountedPartitions = [];
                if (mountedStr !== "Ninguna" && mountedStr !== "") {
                    // --- CORRECCIÓN 3: Usar método .split() del string ---
                    mountedPartitions = mountedStr.split('|');
                    // Limpiar espacios
                    for (let i = 0; i < mountedPartitions.length; i++) {
                        mountedPartitions[i] = mountedPartitions[i].trim();
                    }
                }

                parsedDisks.push({
                    name: diskName,
                    path: diskPath,
                    size: diskSize,
                    fit: diskFit,
                    mountedPartitions: mountedPartitions
                });
            }

            this.disks = parsedDisks;
            console.log("Discos parseados:", this.disks);
        },

        // Método para manejar selección (sin cambios)
        selectDisk(disk) {
            console.log("Disco seleccionado:", disk);
            alert(`Seleccionaste el disco: ${disk.name}\nPath: ${disk.path}\nMontadas: ${disk.mountedPartitions.join(', ') || 'Ninguna'}`);
            // Posible navegación futura:
            // const encodedPath = encodeURIComponent(disk.path);
            // this.$router.push(`/disk-details/${encodedPath}`);
        },

        // Método para volver (sin cambios)
        volverAConsola() {
            console.log("Volviendo a la consola...");
            this.$router.push('/');
        }
    },
    // Hook mounted (sin cambios)
    mounted() {
        console.log("Componente DiskPage montado. Obteniendo discos...");
        this.fetchDisks();
    }
}
</script>

<style scoped>
/* Estilos (sin cambios) */
.disk-icon-wrapper {
    transition: background-color 0.2s ease-in-out, transform 0.2s ease-in-out;
}

.disk-icon-wrapper:hover {
    background-color: #e9ecef !important;
    transform: translateY(-3px);
}

.display-4 {
    font-size: 3.5rem;
}
</style>