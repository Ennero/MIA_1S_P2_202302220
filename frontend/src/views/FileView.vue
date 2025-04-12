<template>
    <div class="container py-5">
        <div class="row justify-content-center">
            <div class="col-md-10 col-lg-8">

                <div class="text-center mb-2">
                    <h1 class="display-6 fw-bold text-primary">Visualizador de Archivo</h1>
                </div>
                <div class="card shadow-sm mb-4">
                    <div class="card-header bg-light d-flex justify-content-between align-items-center">
                        <span>ID Partición: <strong class="text-primary">{{ mountId }}</strong></span>
                        <span class="text-truncate">Archivo: <strong class="text-info">{{ decodedFilePath
                        }}</strong></span>
                    </div>
                    <div class="card-body">

                        <div v-if="isLoading" class="text-center my-4">
                            <div class="spinner-border text-secondary" role="status">
                                <span class="visually-hidden">Cargando...</span>
                            </div>
                            <p>Cargando contenido...</p>
                        </div>

                        <div v-else-if="errorMessage" class="alert alert-danger">
                            {{ errorMessage }}
                        </div>

                        <div v-else>
                            <h5 class="mb-3">Contenido:</h5>
                            <pre
                                class="bg-dark text-light p-3 rounded font-monospace file-content-box">{{ fileContent }}</pre>
                        </div>
                    </div>
                    <div class="card-footer text-center">
                        <button @click="goBackToFileExplorer" class="btn btn-secondary">
                            <i class="bi bi-arrow-left me-2"></i>Volver al Explorador
                        </button>
                    </div>
                </div>
            </div>
        </div>
    </div>
</template>

<script>
export default {
    name: 'FileView',
    props: ['mountId', 'filePathEncoded'], 
    data() {
        return {
            fileContent: '',
            isLoading: false,
            errorMessage: '',
            decodedFilePath: ''
        };
    },
    computed: {
        decodedFilePathComputed() { 
            if (!this.filePathEncoded) return 'Inválido';
            try { return decodeURIComponent(this.filePathEncoded); }
            catch (e) { console.error("Error decodificando path:", e); return 'Error Path'; }
        }
    },
    methods: {
        async fetchFileContent() {

            if (!this.mountId || !this.filePathEncoded) {
                this.errorMessage = "Error interno: Falta ID de montaje o path del archivo.";
                return;
            }
            this.decodedFilePath = this.decodedFilePathComputed;
            if (this.decodedFilePath === 'Error Path') {
                this.errorMessage = "Error: El path del archivo en la URL es inválido.";
                return;
            }

            this.isLoading = true;
            this.errorMessage = '';
            this.fileContent = '';
            console.log(`Enviando comando 'cat -id=${this.mountId} -path="${this.decodedFilePath}"'`);

            // Construir comando cat
            const commandString = `cat -id=${this.mountId} -path="${this.decodedFilePath}"`;

            try {
                const response = await fetch('http://localhost:3001/', { // URL Backend
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ command: commandString }),
                });
                const data = await response.json();

                if (!response.ok || data.error) {
                    const errorMsg = data.error || data.output || `Error HTTP ${response.status}`;
                    throw new Error(`Error obteniendo contenido del archivo: ${errorMsg}`);
                }

                this.fileContent = data.output; 
                console.log("Contenido del archivo recibido.");

            } catch (error) {
                console.error("Error en fetchFileContent:", error);
                this.errorMessage = error.message || "Error de conexión o respuesta inválida.";
            } finally {
                this.isLoading = false;
            }
        },

        goBackToFileExplorer() {
            console.log("Volviendo al explorador...");
            this.$router.go(-1);
        }
    },
    mounted() {
        console.log("Componente FileViewerPage montado.");
        this.fetchFileContent();
    }
}
</script>

<style scoped>
.file-content-box {
    max-height: 60vh;
    overflow-y: auto;
    white-space: pre-wrap;
    word-wrap: break-word;
}

.text-break {
    word-break: break-all;
}
</style>