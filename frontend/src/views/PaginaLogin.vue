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
                <button @click="inicio" class="btn btn-danger">Regresar</button>
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
  name: 'LoginView',
  data() {
    return {
      loginForm: {
        idPartition: '',
        username: '',
        password: '',
        rememberUser: false
      }
    }
  },
  methods: {
    handleLogin() {
      // Aquí iría la lógica de autenticación
      console.log('Intentando iniciar sesión con:', this.loginForm);
      // Enviar datos al backend o realizar validación
    },
    inicio() {
      console.log("Redirigiendo a a la página de inicio...");
      const url = "/"; // URL de inicio de sesión
      this.$router.push(url);
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