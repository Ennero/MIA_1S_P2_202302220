<template>
  <div id="app" class="container">
    <router-view></router-view>
    </div>
</template>

<script>
// Puedes quitar la lógica de data/methods de aquí si todo se movió a ConsoleView
export default {
  name: 'App'
  // Componentes como Navbar/Footer se registrarían aquí si los usas
}
</script>

<style>
/* Estilos globales o del layout principal */
@import "https://cdn.jsdelivr.net/npm/bootstrap@5.3.0/dist/css/bootstrap.min.css";
@import "https://cdn.jsdelivr.net/npm/bootstrap-icons@1.10.0/font/bootstrap-icons.css";
body { background: #f8f9fa; }

.card:hover {
  transform: translateY(-5px);
}

.card {
  transition: transform 0.2s;
}

</style>