<template>
    <div class="container py-4">
        <div class="row justify-content-center">
            <div class="col-md-10 col-lg-12">

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

                <div v-else-if="disks.length > 0" class="row g-3 justify-content-center">
                    <div v-for="disk in disks" :key="disk.path" class="col-6 col-sm-4 col-md-4 text-center">
                        <div class="disk-icon-wrapper p-2 border rounded bg-light shadow-sm" @click="selectDisk(disk)"
                            style="cursor: pointer;">
                            <i class="bi bi-hdd-fill text-secondary display-1"></i>
                            <p class="mt-2 mb-0 fw-bold small text-truncate" :title="disk.name"> <b>{{ disk.name }}</b>
                                <br><b>Fit de la partición:</b> {{ disk.fit }}<br> <b>Tamaño:</b> {{ disk.size }} bytes
                                <br> <b> Particiones montadas: </b>{{ disk.mountedPartitions }} <br> <b> Ruta:</b> {{
                                    disk.path }}
                            </p>
                        </div>
                    </div>
                </div>
                <div v-else class="alert alert-warning text-center">
                    <i class="bi bi-info-circle-fill me-2"></i> No hay discos registrados o no se pudo obtener la
                    información.
                </div>
                <div class="mt-4 text-center">
                    <button @click="regresar" class="btn btn-secondary">
                        <i class="bi bi-arrow-left me-2"></i>Regresar
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
                //alert("Respuesta recibida: " + data.output);
                this.parseAndSetDisks(data.output); // Llamar al parser

            } catch (error) {
                console.error("Error en fetchDisks:", error);
                this.errorMessage = error.message || "Error de conexión o respuesta inválida.";
            } finally {
                this.isLoading = false;
            }
        },

        parseAndSetDisks(outputString) {
            if (!outputString || typeof outputString !== 'string') {
                console.warn("String de salida inválido:", outputString);
                this.errorMessage = "Respuesta inválida del servidor (no es string).";
                this.disks = [];
                return;
            }
            const prefix = "DISKS:\n";
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

            // Separar por punto y coma y quitar espacios
            const diskEntries = dataString.split(';');
            const parsedDisks = [];

            // Iterar sobre cada disco
            for (const entry of diskEntries) {
                const trimmedEntry = entry.trim();
                if (trimmedEntry === "") continue;

                const fields = trimmedEntry.split(',');
                if (fields.length !== 5) {
                    console.warn("Entrada de disco con formato incorrecto (campos != 5), saltando:", entry);
                    continue;
                }

                const diskName = fields[0].trim();
                const diskPath = fields[1].trim();
                //alert(`Path del disco: ${diskPath}`);
                const diskSizeStr = fields[2].trim();
                //alert(`Tamaño del disco: ${diskSizeStr}`);
                const diskFit = fields[3].trim();
                //alert(`Ajuste del disco: ${diskFit}`);
                const mountedStr = fields[4].trim();
                //console.log("Particiones montadas:", mountedStr);

                // Usar parseInt y isNaN ---
                const diskSize = parseInt(diskSizeStr, 10); // Base 10
                if (isNaN(diskSize)) {
                    console.warn(`Tamaño inválido (no es número) para disco '${diskName}', saltando:`, diskSizeStr);
                    continue;
                }
                //alert(`Tamaño del disco: ${diskSize}`);

                // Parsear particiones montadas
                let mountedPartitions = [];
                if (mountedStr !== "Ninguna" && mountedStr !== "") {
                    mountedPartitions = mountedStr.split('|');
                    for (let i = 0; i < mountedPartitions.length; i++) {
                        mountedPartitions[i] = mountedPartitions[i].trim();
                    }
                }

                if (mountedPartitions.length === 0) {
                    mountedPartitions = "Ninguna";
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

        selectDisk(disk) {
            console.log("Disco seleccionado, navegando a particiones:", disk);
            if (!disk || !disk.path) {
                console.error("Datos de disco inválidos para navegar.");
                this.errorMessage = "No se puede mostrar particiones para este disco.";
                return;
            }
            try {
                // Codificar el path del disco para pasarlo como parámetro en la URL
                const encodedPath = encodeURIComponent(disk.path);
                //alert(disk.path)
                // Navegar a la nueva ruta 'PartitionPage', pasando el path codificado
                this.$router.push({ name: 'partitions', params: { diskPathEncoded: encodedPath } });
            } catch (e) {
                console.error("Error en codificación de URL o navegación:", e);
                this.errorMessage = "No se pudo navegar a la vista de particiones.";
            }
        },

        regresar() {
            console.log("Volviendo a la consola...");
            this.$router.push('/loged');
        }
    },
    mounted() {
        console.log("Componente DiskPage montado. Obteniendo discos...");
        this.fetchDisks();
    }
}
</script>

<style scoped>
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