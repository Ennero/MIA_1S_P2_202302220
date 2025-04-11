// src/router/index.js
import { createRouter, createWebHistory } from 'vue-router';
// Importa los componentes que actuarán como "páginas"
// Tu vista principal actual (la consola)
import inicio from '@/views/PaginaInicio.vue'; // <-- ¡NECESITARÁS MOVER TU LÓGICA DE App.vue AQUÍ!
// La nueva vista/ventana que quieres mostrar
import login from '@/views/PaginaLogin.vue'; // <-- ¡ESTE ES EL NUEVO ARCHIVO QUE CREARÁS!

const routes = [
    {
        path: '/', // La ruta raíz mostrará la consola
        name: 'inicio',
        component: inicio
    },
    {
        path: '/login', // La URL para tu nueva ventana/vista
        name: 'login',
        component: login
        // Puedes añadir más rutas aquí
    }
];

const router = createRouter({
    history: createWebHistory(process.env.BASE_URL), // O createWebHashHistory()
    routes
});

export default router;