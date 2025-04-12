
import { createRouter, createWebHistory } from 'vue-router';

import inicio from '@/views/StartPage.vue';

import login from '@/views/LoginPage.vue';
import disk from '@/views/DiskPage.vue'; 
import loged from '@/views/LogedPage.vue';
import partitions from '@/views/PartitionsPage.vue';

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
    },
    {
        path: '/disk',
        name: 'disk',
        component: disk
    },
    {
        path: '/loged',
        name: 'loged',
        component: loged
    },
    { 
        path: '/partitions/:diskPathEncoded',
        name: 'partitions',
        component: partitions,
        props: true
    }

];

const router = createRouter({
    history: createWebHistory(process.env.BASE_URL), 
    routes
});

export default router;