<template>
    <div class="container py-5">
        <div class="row justify-content-center">
            <div class="col-md-10 col-lg-10">

                <div class="text-center mb-4">
                    <h1 class="display-6 fw-bold text-primary">Particiones del Disco</h1>
                    <p class="lead text-muted text-break">Path: <code>{{ decodedDiskPath }}</code></p>
                    <hr class="my-4 text-primary opacity-75">
                </div>

                <div v-if="isLoading" class="text-center">
                    <div class="spinner-border text-primary" role="status">
                        <span class="visually-hidden">Cargando...</span>
                    </div>
                </div>

                <div v-else-if="partitions.length > 0" class="card shadow-sm">
                    <div class="card-header bg-secondary text-white">
                        <i class="bi bi-hdd-rack-fill me-2"></i>Lista de Particiones (Haz clic para explorar, si está
                        montada)
                    </div>
                    <div class="table-responsive">
                        <table class="table table-striped table-hover table-bordered mb-0">
                            <thead class="table-dark">
                                <tr>
                                    <th class="text-center">Nombre</th>
                                    <th class="text-center">Tipo</th>
                                    <th class="text-end">Tamaño (bytes)</th>
                                    <th class="text-end">Inicio (byte)</th>
                                    <th class="text-center">Ajuste</th>
                                    <th class="text-center">Estado</th>
                                    <th class="text-center">ID Montaje</th>
                                </tr>
                            </thead>
                            <tbody>
                                <tr v-for="part in partitions" :key="part.name + '-' + part.start"
                                    @click="selectPartition(part)" :style="part.mountId ? 'cursor: pointer;' : ''"
                                    :title="part.mountId ? 'Clic para explorar' : 'No montada'">
                                    <td class="text-center">{{ part.name || '[Sin Nombre]' }}</td>
                                    <td class="text-center">{{ part.type }}</td>
                                    <td class="text-end">{{ part.size.toLocaleString() }}</td>
                                    <td class="text-end">{{ part.start.toLocaleString() }}</td>
                                    <td class="text-center">{{ part.fit }}</td>
                                    <td class="text-center">{{ part.status }}</td>
                                    <td class="text-center">{{ part.mountId || '-' }}</td>
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
    props: ['diskPathEncoded'],
    data() {
        return {
            partitions: [], // { name, type, size, start, fit, status, mountId }
            isLoading: false,
            errorMessage: '',
            decodedDiskPath: ''
        };
    },
    methods: {
        async fetchPartitions() {
            if (!this.diskPathEncoded) { /* ... */ return; }
            try { this.decodedDiskPath = decodeURIComponent(this.diskPathEncoded); } catch (e) { /* ... */ return; }

            this.isLoading = true;
            this.errorMessage = '';
            this.partitions = [];
            const commandString = `partitions -path="${this.decodedDiskPath}"`;
            console.log(`Enviando comando: ${commandString}`);

            try {
                const response = await fetch('http://localhost:3001/', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ command: commandString }) });
                const data = await response.json();
                if (!response.ok || data.error) {  throw new Error(`Error obteniendo particiones: ${data.error || data.output || 'Error desconocido'}`); }
                console.log("Respuesta Partitions:", data.output);
                this.parseAndSetPartitions(data.output);

            } catch (error) {  this.errorMessage = error.message; }
            finally { this.isLoading = false; }
        },

        // Parsea
        parseAndSetPartitions(outputString) {
            if (!outputString || !outputString.startsWith("PARTITIONS:\n")) { this.errorMessage = "Formato inválido"; return; }
            let dataString = outputString.slice("PARTITIONS:\n".length);
            if (dataString.trim() === "") { this.partitions = []; return; }

            const partitionEntries = dataString.split(';');
            const parsedPartitions = [];

            for (const entry of partitionEntries) {
                const trimmedEntry = entry.trim();
                if (trimmedEntry === "") continue;
                const fields = trimmedEntry.split(',');
                if (fields.length < 7) { // Esperamos al menos 7 ahora
                    console.warn("Entrada partición formato incorrecto (< 7 campos):", entry);
                    continue;
                }

                const partName = fields[0].trim();
                const partType = fields[1].trim();
                const partSizeStr = fields[2].trim();
                const partStartStr = fields[3].trim();
                const partFit = fields[4].trim();
                const partStatus = fields[5].trim();
                const mountId = fields[6].trim(); 

                const partSize = parseInt(partSizeStr, 10);
                const partStart = parseInt(partStartStr, 10);
                if (isNaN(partSize) || isNaN(partStart)) { console.warn(`Datos inválidos p '${partName}'`); continue; }

                parsedPartitions.push({
                    name: partName, type: partType, size: partSize, start: partStart,
                    fit: partFit, status: partStatus,
                    mountId: mountId || null
                });
            }
            this.partitions = parsedPartitions;
            console.log("Particiones parseadas:", this.partitions);
        },

        selectPartition(part) {
            console.log("Partición seleccionada:", part);
            if (!part.mountId) {
                alert(`La partición '${part.name}' no está montada.`);
                return; // No hacer nada si no está montada
            }

            console.log(`Navegando al explorador para partición ID: ${part.mountId}, ruta inicial: /`);
            try {
                // Codificar la ruta raíz '/' para la URL
                const encodedRootPath = encodeURIComponent('/');
                this.$router.push({
                    name: 'FilesPage', // Asegúrate que este sea el 'name' de tu ruta en router/index.js
                    params: {
                        mountId: part.mountId, // Pasar el ID de montaje
                        internalPathEncoded: encodedRootPath // Pasar la ruta raíz codificada
                    }
                });
            } catch (e) {
                console.error("Error al navegar al explorador:", e);
                this.errorMessage = "No se pudo abrir el explorador de archivos.";
            }
        },

        goBack() {
            console.log("Volviendo a la página de discos...");
            this.$router.push('/disk');
        }
    },
    mounted() {
        console.log("Componente PartitionPage montado.");
        this.fetchPartitions();
    }
}
</script>

<style scoped>
/* Añadir cursor pointer a las filas clickables */
tbody tr[style*="cursor: pointer"]:hover {
    background-color: #e9ecef;
    /* Opcional: resaltar al pasar mouse */
}

/* ... (otros estilos sin cambios) ... */
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

.text-break {
    word-break: break-all;
}
</style>