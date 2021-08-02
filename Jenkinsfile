pipeline {
    agent any
    stages {
        stage('env') {
            steps {
                sh 'printenv | sort'
            }
        }
        stage('Hello') {
            steps {
                sh 'echo Hello 5'
                sh 'pwd'
                sh 'ls -l'
            }
        }
        stage('Build') {
            steps {
                sh 'make docker-build'
            }
        }
        stage('Unit Test') {
            steps {
                sh 'make test'
            }
        }
        stage('Docker Test') {
            steps {
                sh 'make docker-test'
            }
        }
        stage('System Test') {
            steps {
                sh 'make system-test'
            }
        }
    }
}

