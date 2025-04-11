// src/main.js
import { createApp } from 'vue';
import App from './App.vue'; // Este será ahora el layout principal
import router from './router'; // Importar el router

// Usar el router en la aplicación
createApp(App).use(router).mount('#app');