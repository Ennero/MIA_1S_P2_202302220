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

                        <div v-if="isLoading" class="text-center my-4">...</div>
                        <div v-else-if="errorMessage" class="alert alert-danger">{{ errorMessage }}</div>

                        <ul v-else-if="items.length > 0" class="list-group list-group-flush">
                            <li
                                class="list-group-item d-flex justify-content-between align-items-center bg-light list-header-custom">
                                <span class="flex-fill fw-bold">Nombre</span>
                                <span class="text-muted small text-end" style="width: 80px;">Tamaño</span>
                                <span class="text-muted small text-end" style="width: 70px;">Perms</span>
                                <span class="text-muted small text-end" style="width: 140px;">Modificado</span>
                            </li>
                            <li v-for="item in sortedItems" :key="item.name"
                                class="list-group-item list-group-item-action d-flex justify-content-between align-items-center"
                                :class="{ 'list-group-item-secondary': item.type === '0', 'clickable': item.type === '0' || item.type === '1' }"
                                @click="handleItemClick(item)"
                                :style="item.type === '0' || item.type === '1' ? 'cursor: pointer;' : ''">

                                <div class="d-flex align-items-center flex-fill me-2 text-truncate">
                                    <i
                                        :class="['bi', item.type === '0' ? 'bi-folder-fill text-warning' : 'bi-file-earmark-text text-info', 'me-2 fs-5']"></i>
                                    <span :class="{ 'fw-bold': item.type === '0' }" :title="item.name">{{ item.name
                                        }}</span>
                                </div>

                                <div class="ms-auto text-muted small d-flex align-items-center text-nowrap">
                                    <span class="me-3 text-end" style="width: 80px;">
                                        {{ item.type === '1' ? formatSize(item.size) : '-' }}
                                    </span>
                                    <span class="me-3 text-end" style="width: 70px;">{{ item.perms }}</span>
                                    <span class="text-end" style="width: 140px;">{{ item.mtime }}</span>
                                </div>
                            </li>
                        </ul>
                        <p v-else class="text-muted text-center my-4">El directorio está vacío.</p>

                    </div>
                    <div class="card-footer text-center"> <button @click="goBackToPartitions" class="btn btn-secondary">
                            Volver </button> </div>
                </div>
            </div>
        </div>
    </div>
</template>

<script>

export default {
    name: 'FilesPage',
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

        decodedInternalPath() {
            if (!this.internalPathEncoded) return '/';
            try {
                let decoded = decodeURIComponent(this.internalPathEncoded);
                if (!decoded.startsWith('/')) decoded = '/' + decoded;
                if (decoded !== '/' && decoded.endsWith('/')) decoded = decoded.slice(0, -1);
                return decoded;
            } catch (e) {
                console.error("Error decodificando path:", e);
                return '/';
            }
        }
    },
    watch: {
        // Observar cambios en el parámetro de la ruta para recargar
        internalPathEncoded(newVal, oldVal) {
            if (newVal !== oldVal) {
                console.log("Watcher: Cambio detectado en internalPathEncoded, recargando contenido...");
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
            console.log(`Enviando comando 'content -id=${this.mountId} -ruta="${pathToList}"'`);
            const commandString = `content -id=${this.mountId} -ruta="${pathToList}"`;

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
                    this.parseAndSetItems(data.output); // Parsea la respuesta
                }

            } catch (error) { /* ... */ this.errorMessage = error.message; }
            finally { this.isLoading = false; }
        },

        parseAndSetItems(outputString) {
            if (!outputString || typeof outputString !== 'string') { return; }
            const prefix = "CONTENT:\n";
            if (!outputString.startsWith(prefix)) { this.errorMessage = "Formato inválido"; return; }
            let dataString = outputString.slice(prefix.length);
            if (dataString.trim() === "" || dataString.includes("está vacío")) { this.items = []; return; }

            const lines = dataString.split('\n');
            const parsedItems = [];

            for (const line of lines) {
                const trimmedLine = line.trim();
                if (trimmedLine === "") continue;
                const fields = trimmedLine.split(',');
                // --- Espera 5 campos ---
                if (fields.length !== 5) {
                    console.warn("Entrada formato incorrecto (campos != 5):", line);
                    continue;
                }
                const itemName = fields[0].trim();
                const itemType = fields[1].trim();
                const itemMtimeStr = fields[2].trim();
                const itemSizeStr = fields[3].trim();
                const itemPerms = fields[4].trim();

                let itemSize = -1;
                if (itemSizeStr !== "-1") {
                    const parsedSize = parseInt(itemSizeStr, 10);
                    if (!isNaN(parsedSize)) { itemSize = parsedSize; }
                    else { console.warn(`Tamaño inválido para '${itemName}': ${itemSizeStr}`); }
                }

                if (itemName === '.' || itemName === '..') { continue; }

                parsedItems.push({
                    name: itemName, type: itemType, mtime: itemMtimeStr,
                    size: itemSize, perms: itemPerms
                });
            }
            this.items = parsedItems;
            console.log("Items parseados:", this.items);
        },

        handleItemClick(item) {
            if (item.type === '0') { this.navigateTo(item); } // Directorio
            else if (item.type === '1') { // Archivo
                console.log(`Navegando al visor para archivo: ${item.name}`);
                const currentDecodedPath = this.decodedInternalPath;
                let filePath = (currentDecodedPath === '/') ? '/' + item.name : currentDecodedPath + '/' + item.name;
                try {
                    const encodedFilePath = encodeURIComponent(filePath);
                    this.$router.push({
                        name: 'FileView', // Nombre ruta visor
                        params: { mountId: this.mountId, filePathEncoded: encodedFilePath }
                    });
                } catch (e) { console.error("Error al navegar al visor:", e); this.errorMessage = "Error al abrir archivo."; }
            }
        },

        navigateTo(item) {
            if (item.type !== '0') return;
            const currentDecodedPath = this.decodedInternalPath;
            let newPath = (currentDecodedPath === '/') ? '/' + item.name : currentDecodedPath + '/' + item.name;
            console.log(`Navegando a subdirectorio: ${newPath}`);
            try {
                const newEncodedPath = encodeURIComponent(newPath);
                this.$router.push({
                    name: 'FilesPage', 
                    params: { mountId: this.mountId, internalPathEncoded: newEncodedPath }
                });
            } catch (e) { console.error("Error al navegar a subdirectorio:", e); this.errorMessage = "Error al navegar."; }
        },

        goUp() {
            const currentDecodedPath = this.decodedInternalPath;
            if (currentDecodedPath === '/') return;
            let lastSlash = currentDecodedPath.lastIndexOf('/');
            let newPath = (lastSlash > 0) ? currentDecodedPath.substring(0, lastSlash) : "/";
            console.log(`Subiendo a directorio padre: ${newPath}`);
            try {
                const newEncodedPath = encodeURIComponent(newPath);
                this.$router.push({
                    name: 'FilesPage',
                    params: { mountId: this.mountId, internalPathEncoded: newEncodedPath }
                });
            } catch (e) { console.error("Error al navegar hacia arriba:", e); this.errorMessage = "Error al subir nivel."; }
        },

        goBackToPartitions() {
            console.log("Volviendo a la selección de particiones...");
            this.$router.go(-1); // Ir atrás en el historial
        },
        formatSize(bytes) {
            if (bytes < 0 || typeof bytes !== 'number' || isNaN(bytes)) return '-';
            if (bytes === 0) return '0 bytes';
            const k = 1024;
            const sizes = ['bytes', 'KB', 'MB', 'GB', 'TB'];
            const i = Math.max(0, Math.floor(Math.log(bytes) / Math.log(k)));
            let num = parseFloat((bytes / Math.pow(k, i)).toFixed(1));
            if (i === 0) { num = Math.round(num); }
            return num + ' ' + sizes[i];
        }


    },
    mounted() {
        console.log("Componente FileExplorerPage montado.");
        this.fetchDirectoryContent();
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