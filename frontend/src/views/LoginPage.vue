<template>
  <div id="app" class="container py-5">
    <div class="row justify-content-center">
      <div class="col-md-6">
        <!-- Cabecera -->
        <div class="text-center mb-4">
          <h1 class="display-5 fw-bold text-primary">Iniciar Sesión</h1>
          <hr class="my-4 text-primary opacity-75">
        </div>

        <!-- Panel de login -->
        <div class="card border-0 shadow-lg mb-4 bg-white rounded">
          <div class="card-header bg-primary text-white p-3">
            <div class="d-flex align-items-center">
              <i class="bi bi-box-arrow-in-right me-2 fs-5"></i>
              <h5 class="mb-0">Acceso al Sistema</h5>
            </div>
          </div>
          <div class="card-body p-4">

            <div v-if="statusMessage" class="alert alert-info py-2">{{ statusMessage }}</div>
            <div v-if="errorMessage" class="alert alert-danger py-2">{{ errorMessage }}</div>

            <form @submit.prevent="handleLogin">
              <!-- ID Partition -->
              <div class="mb-3">
                <label for="idPartition" class="form-label">ID Partition:</label>
                <input type="text" class="form-control bg-light" id="idPartition" v-model="loginForm.idPartition"
                  placeholder="201A" required>
              </div>

              <!-- Usuario -->
              <div class="mb-3">
                <label for="usuario" class="form-label">Usuario:</label>
                <input type="text" class="form-control bg-light" id="usuario" v-model="loginForm.username"
                  placeholder="root" required>
              </div>

              <!-- Contraseña -->
              <div class="mb-3">
                <label for="password" class="form-label">Contraseña:</label>
                <input type="password" class="form-control bg-light" id="password" v-model="loginForm.password"
                  placeholder="********" required>
              </div>

              <!-- Botón de envío -->
              <div class="d-grid gap-2">
                <button type="submit" class="btn btn-primary">Iniciar Sesión</button>
              </div>

              <div class="mt-1 d-grid text-center">
                <button type="button" @click="volverAConsola" class="btn btn-danger">
                  Regresar
                </button>
              </div>

            </form>
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
  name: 'UserLogin',
  data() {
    return {
      loginForm: {
        idPartition: '',
        username: '',
        password: '',
      },
      statusMessage: '',
      errorMessage: ''
    }
  },
  methods: {
    async handleLogin() { // Marcar como async para usar await
      this.errorMessage = "";
      this.statusMessage = "Validando credenciales...";
      console.log('Intentando iniciar sesión con:', this.loginForm);

      const backendUrl = process.env.VUE_APP_BACKEND_URL || 'http://localhost:3001/'; // URL del backend

      if (!this.loginForm.idPartition || !this.loginForm.username || !this.loginForm.password) {
        this.errorMessage = "Todos los campos son obligatorios.";
        this.statusMessage = "";
        return;
      }

      // Construir el string del comando login
      const commandString = `login -user=${this.loginForm.username} -pass=${this.loginForm.password} -id=${this.loginForm.idPartition}`;

      console.log("Enviando comando:", commandString);

      try {
        const response = await fetch(backendUrl, { // URL del backend
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({ command: commandString }), // Enviar el comando como lo espera el backend
        });

        const data = await response.json(); // Esperar y parsear la respuesta JSON

        if (!response.ok || data.error) {
          // Hubo un error HTTP o el backend reportó un error específico
          const errorMsg = data.error || data.output || `Error HTTP ${response.status}`;
          this.errorMessage = `Error de inicio de sesión: ${errorMsg}`;
          this.statusMessage = ""; // Limpiar mensaje de estado
          console.error("Error en login desde backend:", errorMsg);
        } else {
          // Login válido en el backend
          this.statusMessage = "¡Inicio de sesión exitoso! Redirigiendo...";
          console.log("Login successful:", data.output); // Mostrar mensaje del backend (si lo hay)

          setTimeout(() => {
            this.$router.push('/loged'); // Redirigir a la vista de disco después de 1 segundo
          }, 1000); // Espera 1 segundo antes de redirigir

        }

      } catch (error) {
        // Error de red (backend no disponible, CORS, etc.) o error parseando JSON
        console.error("Error en fetch durante login:", error);
        this.errorMessage = "Error de conexión con el servidor. Asegúrate de que el backend esté corriendo.";
        this.statusMessage = "";
      }
    },
    volverAConsola() {
      console.log("Volviendo a la consola...");
      this.$router.push('/'); // Navegar a la ruta raíz
    }
  }
}
</script>

<style scoped>
input.form-control:focus {
  box-shadow: 0 0 0 0.25rem rgba(13, 110, 253, 0.25);
  border-color: #86b7fe;
}
</style>