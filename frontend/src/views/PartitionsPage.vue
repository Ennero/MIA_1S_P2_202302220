<template>
    <div class="container py-5">
        <div class="row justify-content-center">
            <div class="col-md-10 col-lg-10"> 

                <div class="text-center mb-4">
                    <h1 class="display-6 fw-bold text-primary">Particiones del Disco</h1>
                    <p class="lead text-muted text-break">Path: <code>{{ decodedDiskPath }}</code></p>
                    <hr class="my-4 text-primary opacity-75">
                </div>

                <div v-if="isLoading" class="text-center my-5">
                    <div class="spinner-border text-primary" role="status">
                        <span class="visually-hidden">Cargando...</span>
                    </div>
                    <p class="mt-2">Cargando particiones...</p>
                </div>

                <div v-else-if="errorMessage" class="alert alert-danger text-center">
                    <i class="bi bi-exclamation-triangle-fill me-2"></i> {{ errorMessage }}
                </div>

                <div v-else-if="partitions.length > 0" class="card shadow-sm">
                    <div class="card-header bg-secondary text-white">
                        <i class="bi bi-hdd-rack-fill me-2"></i>Lista de Particiones Encontradas
                    </div>
                    <div class="table-responsive">
                        <table class="table table-striped table-hover table-bordered mb-0">
                            <thead class="table-dark"> 
                                <tr>
                                    <th>Nombre</th>
                                    <th>Tipo</th>
                                    <th>Tamaño (bytes)</th>
                                    <th class="text-center">Ajuste</th>
                                    <th class="text-center">Estado</th>
                                </tr>
                            </thead>
                            <tbody>
                                <tr v-for="part in partitions" :key="part.name + '-' + part.start">
                                    <td>{{ part.name || '[Sin Nombre]' }}</td>
                                    <td class="text-center">{{ part.type }}</td>
                                    <td class="text-end">{{ part.size.toLocaleString() }}</td>
                                    <td class="text-center">{{ part.fit }}</td>
                                    <td class="text-center">{{ part.status }}</td>
                                </tr>
                            </tbody>
                        </table>
                    </div>
                </div>

                <div v-else class="alert alert-secondary text-center">
                    <i class="bi bi-info-circle-fill me-2"></i> No se encontraron particiones válidas en este disco o el
                    disco está vacío.
                </div>

                <div class="mt-4 text-center">
                    <button @click="goBack" class="btn btn-secondary">
                        <i class="bi bi-arrow-left me-2"></i>Volver a Selección de Discos
                    </button>
                </div>

            </div>
        </div>
    </div>
</template>

<script>
export default {
    name: 'PartitionPage',
    props: ['diskPathEncoded'], // Recibe el parámetro codificado de la ruta
    data() {
        return {
            partitions: [],    // Array para { name, type, size, start, fit, status }
            isLoading: false,
            errorMessage: '',
            decodedDiskPath: '' // Para mostrar el path en la UI
        };
    },
    methods: {
        async fetchPartitions() {
            // Validar que se recibe el prop
            if (!this.diskPathEncoded) {
                this.errorMessage = "Error interno: No se recibió la ruta del disco.";
                console.error("diskPathEncoded prop está vacío.");
                return;
            }

            try {
                this.decodedDiskPath = decodeURIComponent(this.diskPathEncoded);
                console.log("Path decodificado:", this.decodedDiskPath);
            } catch (e) {
                this.errorMessage = "Error: El path del disco en la URL es inválido.";
                console.error("Error decodificando URI:", e);
                return;
            }

            this.isLoading = true;
            this.errorMessage = '';
            this.partitions = [];
            console.log(`Enviando comando 'partitions -path="${this.decodedDiskPath}"'`);

            // Construir comando asegurando comillas por si el path tiene espacios
            const commandString = `partitions -path="${this.decodedDiskPath}"`;

            try {
                const response = await fetch('http://localhost:3001/', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ command: commandString }),
                });
                const data = await response.json();

                if (!response.ok || data.error) {
                    const errorMsg = data.error || data.output || `Error HTTP ${response.status}`;
                    // Si el error es que no se encontraron particiones, mostrar mensaje amigable
                    if (typeof data.output === 'string' && data.output.includes("No se encontraron particiones")) {
                        this.errorMessage = ''; // No es un error, solo no hay particiones
                        this.partitions = [];
                        console.log("Respuesta indica que no hay particiones válidas.");
                    } else {
                        throw new Error(`Error obteniendo particiones: ${errorMsg}`);
                    }
                } else {
                    console.log("Respuesta Partitions:", data.output);
                    this.parseAndSetPartitions(data.output); // Llamar al parser
                }

            } catch (error) {
                console.error("Error en fetchPartitions:", error);
                this.errorMessage = error.message || "Error de conexión o respuesta inválida.";
            } finally {
                this.isLoading = false;
            }
        },

        // Parsea el string
        parseAndSetPartitions(outputString) {
            if (!outputString || typeof outputString !== 'string') { this.errorMessage = "NO debería de dar error pero por si acaso" ; return; }
            const prefix = "PARTITIONS:\n"; // Prefijo esperado
            if (!outputString.startsWith(prefix)) { this.errorMessage = "No se encuentra el prefiojo esperado" ; return}
            let dataString = outputString.slice(prefix.length);
            if (dataString.trim() === "") { this.partitions = []; return; }

            const partitionEntries = dataString.split(';');
            const parsedPartitions = [];

            for (const entry of partitionEntries) {
                const trimmedEntry = entry.trim();
                if (trimmedEntry === "") continue;
                const fields = trimmedEntry.split(',');
                if (fields.length !== 6) {
                    console.warn("Entrada de partición formato incorrecto (campos != 6):", entry);
                    continue;
                }

                // Extraer y limpiar cada campo
                const partName = fields[0].trim();
                const partType = fields[1].trim();
                const partSizeStr = fields[2].trim();
                const partStartStr = fields[3].trim();
                const partFit = fields[4].trim();
                const partStatus = fields[5].trim();

                // Convertir size y start a números
                const partSize = parseInt(partSizeStr, 10);
                const partStart = parseInt(partStartStr, 10);

                if (isNaN(partSize) || isNaN(partStart)) {
                    console.warn(`Datos inválidos (size/start) para partición '${partName}', saltando:`, entry);
                    continue;
                }

                parsedPartitions.push({
                    name: partName,
                    type: partType,
                    size: partSize,
                    start: partStart,
                    fit: partFit,
                    status: partStatus
                });
            }
            this.partitions = parsedPartitions;
            console.log("Particiones parseadas:", this.partitions);
        },

        // Volver a la página anterior 
        goBack() {
            console.log("Volviendo a la página de discos...");
            this.$router.push('/disk'); 
        }
    },
    // Llamar a fetchPartitions cuando el componente se monta
    mounted() {
        console.log("Componente PartitionPage montado.");
        this.fetchPartitions();
    }
}
</script>

<style scoped>
/* Estilos específicos para la tabla o la página */
.table th {
    background-color: #343a40;
    color: white;
}

.text-end {
    text-align: right;
}

.text-center {
    text-align: center;
}

/* Para evitar que paths largos rompan el layout */
.text-break {
    word-break: break-all;
}
</style>