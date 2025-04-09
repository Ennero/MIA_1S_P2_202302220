<template>
  <div id="app" class="container py-5">
    <div class="row justify-content-center">
      <div class="col-md-10">
        <!-- Cabecera -->
        <div class="text-center mb-4">
          <h1 class="display-5 fw-bold text-primary">Sistema de archivos EXT2</h1>
          <hr class="my-4 text-primary opacity-75">
        </div>
        
        <!-- Panel de comandos -->
        <div class="card border-0 shadow-lg mb-4 bg-white rounded">
          <div class="card-header bg-primary text-white p-3">
            <div class="d-flex align-items-center">
              <i class="bi bi-terminal-fill me-2 fs-5"></i>
              <h5 class="mb-0">Consola de comandos</h5>
            </div>
          </div>
          <div class="card-body p-4">
            <div class="form-floating mb-3">
              <textarea 
                v-model="entrada" 
                class="form-control bg-light"
                id="commandTextarea" 
                style="height: 120px"
                placeholder="Escribe comandos aquí..."
              ></textarea>
              <label for="commandTextarea">Ingresa el comando o script</label>
            </div>
            
            <div class="row g-3">
              <div class="col-md-6">
                <div class="input-group">
                  <label class="input-group-text bg-secondary text-white">
                    <i class="bi bi-file-earmark-text"></i>
                  </label>
                  <input 
                    type="file" 
                    class="form-control" 
                    @change="handleFileUpload"
                    accept=".mias"
                    id="fileInput"
                  />
                </div>
                <div class="form-text text-muted mt-1">
                  <i class="bi bi-info-circle-fill me-1"></i> Solo archivos con extensión .mias
                </div>
                <div v-if="fileError" class="alert alert-danger mt-2 py-2 small">
                  <i class="bi bi-exclamation-triangle-fill me-1"></i> {{ fileError }}
                </div>
              </div>
              <div class="col-md-3">
                <button class="btn btn-primary w-100 d-flex justify-content-center align-items-center" @click="ejecutar">
                  <i class="bi bi-play-fill me-2"></i> Ejecutar
                </button>
              </div>
              <div class="col-md-3">
                <button class="btn btn-danger w-100 d-flex justify-content-center align-items-center" @click="limpiar">
                  <i class="bi bi-trash me-2"></i> Limpiar
                </button>
              </div>
            </div>
          </div>
        </div>
        
        <!-- Panel de salida -->
        <div class="card border-0 shadow-lg bg-white rounded">
          <div class="card-header bg-success text-white p-3">
            <div class="d-flex align-items-center">
              <i class="bi bi-code-square me-2 fs-5"></i>
              <h5 class="mb-0">Resultado de comandos</h5>
            </div>
          </div>
          <div class="card-body p-4">
            <!-- Se eliminó form-floating y su label -->
            <textarea 
              v-model="salida" 
              class="form-control bg-dark text-light font-monospace"
              style="height: 180px"
              id="outputTextarea"
              readonly
              placeholder="La salida aparecerá aquí..."
            ></textarea>
          </div>
          <div class="card-footer bg-light p-3 text-end">
            <span class="badge bg-info text-dark">
              <i class="bi bi-info-circle me-1"></i> Sistema de archivos EXT2 • Enner Mendizabal 202302220
            </span>
          </div>
        </div>

      </div>
    </div>
  </div>
</template>

<script>
export default {
  data() {
    return {
      entrada: "",
      salida: "",
      fileError: ""
    };
  },
  methods: {
    handleFileUpload(event) {
      const file = event.target.files[0];
      this.fileError = "";

      if (!file) return;

      const fileName = file.name;
      const fileExtension = fileName.split('.').pop().toLowerCase();

      if (fileExtension !== 'mias') {
        this.fileError = "Solo se permiten archivos con extensión .mias";
        event.target.value = '';
        return;
      }

      const reader = new FileReader();
      reader.onload = (e) => {
        this.entrada = e.target.result;
        this.salida = `✅ Archivo cargado: ${fileName}\n--- Contenido ---\n${this.entrada}\n---------------\nListo para ejecutar.`; // Mostrar contenido cargado
      };
      reader.readAsText(file);
    },
    async ejecutar() { // Marcar la función como async
      if (!this.entrada.trim()) {
        this.salida = "⚠️ No hay comandos para ejecutar";
        return;
      }

      // Limpiar salida anterior e indicar inicio
      this.salida = "🔄 Ejecutando comandos...\n------------------------\n";

      // Dividir la entrada en líneas
      const lines = this.entrada.split('\n');
      let hasErrors = false; // Para rastrear si hubo algún error

      // Iterar sobre cada línea
      for (const line of lines) {
        const trimmedLine = line.trim(); // Quitar espacios inicio/fin

        // Ignorar líneas vacías y comentarios en el frontend también
        if (trimmedLine === "" || trimmedLine.startsWith("#")) {
          // Opcional: Mostrar línea ignorada en la salida
          // this.salida += `(Ignorando: ${line})\n`;
          continue; // Pasar a la siguiente línea
        }

        // Mostrar el comando que se va a ejecutar
        this.salida += `> ${trimmedLine}\n`;

        try {
          const response = await fetch('http://localhost:3001/', { // Usar await para esperar la respuesta
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
            },
            // Enviar solo la línea actual como comando
            body: JSON.stringify({ command: trimmedLine }),
          });

          // Leer la respuesta del backend
          const data = await response.json(); // Usar await

          // Verificar si el backend reportó un error en su estructura JSON
          if (data.error) {
              this.salida += `❌ Error: ${data.error}\n`;
              hasErrors = true;
          } else if (!response.ok) {
              let errorMsg = `Error HTTP ${response.status}`;
             if(data.output) { // Si hay 'output' aunque no sea OK, podría tener el error
                errorMsg += `: ${data.output}`;
             } else if(data.error) { // O si hay campo 'error'
                errorMsg += `: ${data.error}`;
              }
              this.salida += `❌ ${errorMsg}\n`;
              hasErrors = true;
          } else {
            if (data.output && data.output.trim() !== "") {
                this.salida += `${data.output}\n`;
            } else {
                 // Si no hay output, al menos indicar que se completó ok (opcional)
                this.salida += `(OK)\n`;
            }
          }
        } catch (error) {
           // Error de red o al parsear JSON
          console.error("Error en fetch:", error);
          this.salida += `❌ Error de conexión o respuesta inválida del backend.\n`;
          hasErrors = true;
        }
        this.salida += "------------------------\n";
      } 

      this.salida += hasErrors ? "⚠️ Ejecución completada con errores." : "✅ Ejecución completada.";

    },
    limpiar() {
       // ... (tu código de limpiar sin cambios)
      this.entrada = "";
      this.salida = "";
      this.fileError = "";
      const fileInput = document.getElementById('fileInput');
      if (fileInput) fileInput.value = '';
    },
  },
};
</script>

<style>
@import "https://cdn.jsdelivr.net/npm/bootstrap@5.3.0/dist/css/bootstrap.min.css";
@import "https://cdn.jsdelivr.net/npm/bootstrap-icons@1.10.0/font/bootstrap-icons.css";

body {
  background: #f8f9fa;
}
.card {
  transition: transform 0.2s;
}

.card:hover {
  transform: translateY(-5px);
}

textarea.form-control:focus {
  box-shadow: 0 0 0 0.25rem rgba(13, 110, 253, 0.25);
  border-color: #86b7fe;
}

.font-monospace {
  font-family: SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
}
</style>