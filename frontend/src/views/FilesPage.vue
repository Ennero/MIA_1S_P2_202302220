<template>
    <div class="container py-5">
        <div class="row justify-content-center">
            <div class="col-md-10 col-lg-10">

                <div class="text-center mb-2">
                    <h1 class="display-6 fw-bold text-primary">Explorador de Archivos</h1>
                </div>
                <div class="card shadow-sm mb-4">
                    <div class="card-header bg-light d-flex justify-content-between align-items-center">
                        <span>ID Partición: <strong class="text-primary">{{ mountId }}</strong></span>
                        <span>Ruta Actual: <strong class="text-success">{{ decodedInternalPath }}</strong></span>
                    </div>
                    <div class="card-body">
                        <button class="btn btn-sm btn-outline-secondary mb-3" @click="goUp"
                            :disabled="decodedInternalPath === '/' || isLoading">
                            <i class="bi bi-arrow-up-circle me-1"></i> Subir Nivel (..)
                        </button>

                        <div v-if="isLoading" class="text-center my-4">
                            <div class="spinner-border text-secondary" role="status">
                                <span class="visually-hidden">Cargando...</span>
                            </div>
                        </div>
                        <div v-else-if="errorMessage" class="alert alert-danger">
                            {{ errorMessage }}
                        </div>


                        <ul v-else-if="items.length > 0" class="list-group list-group-flush">
                            <li v-for="item in sortedItems" :key="item.name"
                                class="list-group-item list-group-item-action d-flex align-items-center"
                                :class="{ 'list-group-item-secondary': item.type === '0', 'fw-bold': item.type === '0', 'clickable': item.type === '0' }"
                                @click="item.type === '0' ? navigateTo(item) : null"
                                :style="item.type === '0' ? 'cursor: pointer;' : ''">

                                <i
                                    :class="['bi', item.type === '0' ? 'bi-folder-fill text-warning' : 'bi-file-earmark-text text-info', 'me-3 fs-5']"></i>
                                <span>{{ item.name }}</span>
                            </li>
                        </ul>

                        <p v-else class="text-muted text-center my-4">El directorio está vacío.</p>


                    </div>
                    <div class="card-footer text-center">
                        <button @click="goBackToPartitions" class="btn btn-secondary">
                            <i class="bi bi-arrow-left me-2"></i>Volver a Particiones
                        </button>
                    </div>
                </div>
            </div>
        </div>
    </div>
</template>

<script>

export default {
    name: 'FileExplorerPage',
    props: ['mountId', 'internalPathEncoded'],
    data() {
        return {
            items: [],
            isLoading: false,
            errorMessage: '',
        };
    },
    computed: {
        sortedItems() {
            return [...this.items].sort((a, b) => {
                if (a.name === '.') return -1; if (b.name === '.') return 1;
                if (a.name === '..') return -1; if (b.name === '..') return 1;
                if (a.type !== b.type) { return a.type < b.type ? -1 : 1; }
                return a.name.toLowerCase().localeCompare(b.name.toLowerCase());
            });
        },
        // Propiedad computada para obtener el path decodificado
        decodedInternalPath() {
            if (!this.internalPathEncoded) return '/';
            try {
                let decoded = decodeURIComponent(this.internalPathEncoded);
                if (!decoded.startsWith('/')) decoded = '/' + decoded;
                if (decoded !== '/' && decoded.endsWith('/')) decoded = decoded.slice(0, -1);
                return decoded;
            } catch (e) {
                console.error("Error decodificando path:", e);
                return '/'; // Retornar '/' en caso de error
            }
        }
    },
    watch: {
        // Observar cambios en el parámetro de la ruta para recargar
        internalPathEncoded(newVal, oldVal) {
            if (newVal !== oldVal) {
                console.log("Watcher: Cambio detectado en internalPathEncoded, recargando contenido...");
                // Ya no necesitamos actualizar this.currentPath aquí
                this.fetchDirectoryContent();
            }
        }
    },
    methods: {
        // Carga el contenido del directorio actual
        async fetchDirectoryContent() {
            this.isLoading = true;
            this.errorMessage = '';
            this.items = [];

            const pathToList = this.decodedInternalPath;
            // Construcción del comando:
            console.log(`Enviando comando 'content -id=${this.mountId} -ruta="${pathToList}"'`); // <-- YA USA mountId
            const commandString = `content -id=${this.mountId} -ruta="${pathToList}"`;          // <-- YA USA mountId

            try {
                // Envío del comando al backend
                const response = await fetch('http://localhost:3001/', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ command: commandString }) });
                const data = await response.json();

                // Manejo de respuesta (asumiendo que el backend devuelve "nombre,tipo\n...")
                if (!response.ok || data.error) {
                    // ... manejo de error ...
                    const errorMsg = data.error || data.output || `Error HTTP ${response.status}`;
                    // Manejar caso específico de directorio vacío como no-error
                    if (typeof data.output === 'string' && data.output.includes("Directorio") && data.output.includes("está vacío")) {
                        this.errorMessage = ''; this.items = []; console.log(`Directorio '${pathToList}' está vacío.`);
                    } else { throw new Error(`Error obteniendo contenido: ${errorMsg}`); }
                } else {
                    this.parseAndSetItems(data.output); // Parsea la respuesta "nombre,tipo\n..."
                }

            } catch (error) { /* ... */ this.errorMessage = error.message; }
            finally { this.isLoading = false; }
        },

        parseAndSetItems(outputString) {
            // ... (lógica de parseo para "nombre,tipo\n...") ...
            if (!outputString || typeof outputString !== 'string') { return; }
            const prefix = "CONTENT:\n";
            if (!outputString.startsWith(prefix)) {alert('hola'); return; }
            let dataString = outputString.slice(prefix.length);
            if (dataString.trim() === "" || dataString.includes("está vacío")) { this.items = []; return; }
            const lines = dataString.split('\n'); const parsedItems = [];
            for (const line of lines) {
                const trimmedLine = line.trim();
                if (trimmedLine === "") continue;
                const fields = trimmedLine.split(',');
                if (fields.length !== 2) { continue; } const itemName = fields[0].trim();
                const itemType = fields[1].trim();
                if (itemName === '.' || itemName === '..') { continue; } parsedItems.push({ name: itemName, type: itemType });
            } this.items = parsedItems; console.log("Items parseados:", this.items);

        },

        // Modificado para usar decodedInternalPath
        navigateTo(item) {
            if (item.type !== '0') return;
            console.log(`Navegando a directorio: ${item.name}`);
            const currentDecodedPath = this.decodedInternalPath;
            let newPath;
            if (currentDecodedPath === '/') { newPath = '/' + item.name; }
            else { newPath = currentDecodedPath + '/' + item.name; }
            try {
                const newEncodedPath = encodeURIComponent(newPath);
                this.$router.push({ name: 'FileExplorer', params: { mountId: this.mountId, internalPathEncoded: newEncodedPath } });
            }
            catch (e) { console.error("Error al navegar a subdirectorio:", e); this.errorMessage = "Error al navegar."; }
        },

        goUp() {
            const currentDecodedPath = this.decodedInternalPath; // Usar la computada
            if (currentDecodedPath === '/') return;
            let lastSlash = currentDecodedPath.lastIndexOf('/');
            let newPath = (lastSlash > 0) ? currentDecodedPath.substring(0, lastSlash) : "/";
            console.log(`Subiendo a directorio padre: ${newPath}`);
            try { const newEncodedPath = encodeURIComponent(newPath); this.$router.push({ name: 'FileExplorer', params: { mountId: this.mountId, internalPathEncoded: newEncodedPath } }); }
            catch (e) { console.error("Error al navegar hacia arriba:", e); this.errorMessage = "Error al subir nivel."; }
        },

        goBackToPartitions() {
            console.log("Volviendo a la selección de particiones...");
            this.$router.go(-1); // Ir atrás en el historial
        }
    },
    mounted() {
        console.log("Componente FileExplorerPage montado.");
        this.fetchDirectoryContent(); // fetchDirectoryContent usará this.decodedInternalPath
    }
}
</script>

<style scoped>
.table th {
    background-color: #e9ecef;
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

.clickable {
    cursor: pointer;
}

.list-group-item-action:hover {
    background-color: #f8f9fa;
}
</style>